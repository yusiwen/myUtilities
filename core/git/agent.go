package git

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/briandowns/spinner"
	"github.com/yusiwen/myUtilities/core/openai"
	"github.com/yusiwen/myUtilities/core/scip"
	"github.com/yusiwen/myUtilities/core/term"
	xterm "golang.org/x/term"
)

const reviewAgentPrompt = `You are a senior software engineer conducting a thorough code review on a git diff.

You have access to tools to gather more context from the codebase. Use them strategically:

1. Start by reviewing the diff stat and file list provided below
2. Use read_diff to see actual changes for files you want to examine
3. Use read_file to get surrounding context for changed functions
4. Use search_code to find related code or usages
5. Use read_function to get the full function body around a changed line
6. Once you have enough information, produce your final review without calling any tools

Your final review must include these sections:

## Changes Overview
Brief summary of what changes this diff introduces and the overall purpose.

## File-by-File Analysis
For each changed file: what changed, why, and potential impact on the codebase.

## Issues and Concerns
Potential bugs, security risks, performance issues, maintainability improvements, or best practice violations.

## Positive Observations
Well-structured changes, good naming, proper error handling, effective patterns.

Be specific — reference code snippets with line numbers when discussing issues.`

type ReviewAgent struct {
	client         *openai.Client
	messages       []openai.Message
	tools          []openai.ToolDef
	maxTurns       int
	verbose        bool
	progressWriter io.Writer
	diffArgs       []string
	spinner        *spinner.Spinner
	indexSet       *scip.IndexSet
}

type AgentResult struct {
	Content    string
	Turns      int
	MaxReached bool
}

func NewReviewAgent(client *openai.Client, diff *DiffResult, diffArgs []string, lang, context, repoName, branchName, commitHash string, maxTurns int, verbose bool, indexSet *scip.IndexSet) (*ReviewAgent, error) {
	nameStatus, _ := GetNameStatus(diffArgs)

	var initMsg strings.Builder
	initMsg.WriteString(fmt.Sprintf("## Changes Overview\n\nProject: %s\nBranch: %s\nCommit: %s\n\n", repoName, branchName, commitHash))
	initMsg.WriteString("### Diff Stat\n\n```\n")
	initMsg.WriteString(term.StripANSI(diff.Stat))
	initMsg.WriteString("\n```\n\n")

	if nameStatus != "" {
		initMsg.WriteString("### Changed Files\n\n```\n")
		initMsg.WriteString(nameStatus)
		initMsg.WriteString("\n```\n\n")
	}

	untracked := GetUntrackedFiles()
	if len(untracked) > 0 {
		initMsg.WriteString("### Untracked Files\n\n")
		initMsg.WriteString("The following files are new (not yet tracked by git). ")
		initMsg.WriteString("Use **read_file** to view them. search_code and read_diff will not find them.\n\n")
		initMsg.WriteString("```\n")
		for _, f := range untracked {
			initMsg.WriteString(f + "\n")
		}
		initMsg.WriteString("```\n\n")
	}

	for _, name := range []string{"CODEBASE.md", "AGENTS.md", "CONTRIBUTING.md"} {
		data, err := os.ReadFile(name)
		if err != nil {
			continue
		}
		content := strings.TrimSpace(string(data))
		if content == "" {
			continue
		}
		const maxCtxLen = 50000
		if len([]rune(content)) > maxCtxLen {
			content = string([]rune(content)[:maxCtxLen]) + "\n...(truncated at 50000 chars)"
		}
		initMsg.WriteString(fmt.Sprintf("### %s\n\n```\n%s\n```\n\n", name, content))
	}

	if context != "" {
		initMsg.WriteString("### Additional Context\n\n")
		initMsg.WriteString(context)
		initMsg.WriteString("\n\n")
	}

	if indexSet != nil {
		initMsg.WriteString("### Semantic Code Intelligence\n\n")
		initMsg.WriteString("A SCIP semantic index is available. Use **find_definition** and **find_references** to resolve symbols precisely across the repository (find_references is especially useful to assess the impact of signature changes or deletions). Use **symbol_info** for signatures and docs. Lines are 1-based.\n\n")
	}

	initMsg.WriteString("Review the changes above. Use the available tools to read diffs and examine files, then produce your final review.")

	sysPrompt := reviewAgentPrompt
	switch lang {
	case "cn":
		sysPrompt += "\n\nLanguage: Write the review in Chinese (Simplified Chinese). Section titles must be in the same language."
	default:
		sysPrompt += "\n\nLanguage: Write the review in English. Section titles must be in the same language."
	}

	messages := []openai.Message{
		{Role: "system", Content: sysPrompt},
		{Role: "user", Content: initMsg.String()},
	}

	tools := []openai.ToolDef{
		{
			Type: "function",
			Function: openai.ToolFunction{
				Name:        "read_file",
				Description: "Read a file from the working tree. Omit start_line/end_line to read the whole file. For large files, specify a line range.",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"path":       map[string]any{"type": "string", "description": "File path relative to repo root"},
						"start_line": map[string]any{"type": "integer", "description": "First line to read (1-based, inclusive)"},
						"end_line":   map[string]any{"type": "integer", "description": "Last line to read (inclusive)"},
					},
					"required": []string{"path"},
				},
			},
		},
		{
			Type: "function",
			Function: openai.ToolFunction{
				Name:        "read_diff",
				Description: "Get the diff content for a specific file. Call without path to get the full diff.",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"path": map[string]any{"type": "string", "description": "File path to get diff for (omit for full diff)"},
					},
				},
			},
		},
		{
			Type: "function",
			Function: openai.ToolFunction{
				Name:        "search_code",
				Description: "Search for a pattern in tracked files using git grep. Does NOT find untracked files — use read_file for those.",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"pattern": map[string]any{"type": "string", "description": "Search pattern (regex supported)"},
						"path":    map[string]any{"type": "string", "description": "Limit search to a specific file or directory"},
					},
					"required": []string{"pattern"},
				},
			},
		},
		{
			Type: "function",
			Function: openai.ToolFunction{
				Name:        "read_function",
				Description: "Read the function body containing the given line number. Returns a window around the line with function context.",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"path": map[string]any{"type": "string", "description": "File path relative to repo root"},
						"line": map[string]any{"type": "integer", "description": "Line number to get function context for (1-based)"},
					},
					"required": []string{"path", "line"},
				},
			},
		},
	}

	if indexSet != nil {
		tools = append(tools, []openai.ToolDef{
			{
				Type: "function",
				Function: openai.ToolFunction{
					Name:        "find_references",
					Description: "Find all usages/call sites of the symbol at the given line across the whole repository (including other files). Use this to assess the impact of changing or deleting a function, method, or type.",
					Parameters: map[string]any{
						"type": "object",
						"properties": map[string]any{
							"path": map[string]any{"type": "string", "description": "File path relative to repo root"},
							"line": map[string]any{"type": "integer", "description": "1-based line number of the symbol"},
						},
						"required": []string{"path", "line"},
					},
				},
			},
			{
				Type: "function",
				Function: openai.ToolFunction{
					Name:        "find_definition",
					Description: "Jump to the definition of the symbol(s) referenced on the given line. Returns file:line locations and symbol names. Use this to understand what a referenced function or type is.",
					Parameters: map[string]any{
						"type": "object",
						"properties": map[string]any{
							"path": map[string]any{"type": "string", "description": "File path relative to repo root"},
							"line": map[string]any{"type": "integer", "description": "1-based line number of the symbol usage"},
						},
						"required": []string{"path", "line"},
					},
				},
			},
			{
				Type: "function",
				Function: openai.ToolFunction{
					Name:        "symbol_info",
					Description: "Return hover-style information (signature, kind, doc comment) for the symbol at the given line.",
					Parameters: map[string]any{
						"type": "object",
						"properties": map[string]any{
							"path": map[string]any{"type": "string", "description": "File path relative to repo root"},
							"line": map[string]any{"type": "integer", "description": "1-based line number of the symbol"},
						},
						"required": []string{"path", "line"},
					},
				},
			},
		}...)
	}

	var spin *spinner.Spinner
	if xterm.IsTerminal(int(os.Stderr.Fd())) {
		spin = spinner.New(spinner.CharSets[14], 100*time.Millisecond)
		spin.Color("fgHiWhite")
		spin.Suffix = ""
		spin.Writer = os.Stderr
	}

	return &ReviewAgent{
		client:         client,
		messages:       messages,
		tools:          tools,
		maxTurns:       maxTurns,
		verbose:        verbose,
		progressWriter: os.Stderr,
		diffArgs:       diffArgs,
		spinner:        spin,
		indexSet:       indexSet,
	}, nil
}

func (a *ReviewAgent) progressf(format string, args ...any) {
	a.stopSpinner()
	msg := fmt.Sprintf(format, args...)
	fmt.Fprintln(a.progressWriter, term.Faint(msg))
}

func (a *ReviewAgent) startSpinner() {
	if a.spinner != nil {
		a.spinner.Start()
	}
}

func (a *ReviewAgent) stopSpinner() {
	if a.spinner != nil {
		a.spinner.Stop()
	}
}

func (a *ReviewAgent) Run() (*AgentResult, error) {
	statLine := PlainDiffStat(a.diffArgs)
	if statLine == "" {
		statLine = "no changes"
	}
	a.progressf("Reviewing diff: %s", statLine)

	a.startSpinner()
	for turn := 1; turn <= a.maxTurns; turn++ {
		resp, err := a.client.ChatWithTools(a.messages, a.tools)
		if err != nil {
			return nil, fmt.Errorf("agent error at turn %d: %w", turn, err)
		}

		if len(resp.ToolCalls) == 0 {
			a.progressf("Step %d:", turn)
			a.progressf("  Producing final review (%d chars)", len(resp.Content))
			a.progressf("Complete: %d rounds", turn)
			return &AgentResult{Content: resp.Content, Turns: turn}, nil
		}

		asstMsg := openai.Message{
			Role:      "assistant",
			Content:   resp.Content,
			ToolCalls: resp.ToolCalls,
		}
		a.messages = append(a.messages, asstMsg)

		a.progressf("Step %d:", turn)
		for _, tc := range resp.ToolCalls {
			args := formatToolArgs(tc.Function.Name, tc.Function.Arguments)
			a.progressf("  %s(%s)", tc.Function.Name, args)
			result := a.executeTool(tc)
			a.messages = append(a.messages, openai.Message{
				Role:       "tool",
				ToolCallID: tc.ID,
				Content:    truncateToolResult(result),
			})
		}
		a.startSpinner()
	}

	a.stopSpinner()
	a.messages = append(a.messages, openai.Message{
		Role:    "system",
		Content: "You have reached the maximum number of tool calls. Produce your final review now using only the information you have gathered so far. Do not make additional tool calls.",
	})

	resp, err := a.client.ChatWithTools(a.messages, nil)
	if err != nil {
		return nil, err
	}

	if resp.Content == "" {
		a.progressf("Complete: %d rounds (max reached)", a.maxTurns)
		return &AgentResult{Content: "\n\n> ⚠ Review reached maximum turns. Some details may be incomplete.\n", Turns: a.maxTurns, MaxReached: true}, nil
	}

	a.progressf("Complete: %d rounds (max reached)", a.maxTurns)
	return &AgentResult{
		Content:    resp.Content + "\n\n> ⚠ Review reached maximum turns (" + fmt.Sprintf("%d", a.maxTurns) + "). Some details may be incomplete.\n",
		Turns:      a.maxTurns,
		MaxReached: true,
	}, nil
}

func (a *ReviewAgent) executeTool(tc openai.ToolCall) string {
	switch tc.Function.Name {
	case "read_file":
		var args struct {
			Path      string `json:"path"`
			StartLine int    `json:"start_line"`
			EndLine   int    `json:"end_line"`
		}
		if err := json.Unmarshal([]byte(tc.Function.Arguments), &args); err != nil {
			return fmt.Sprintf("error parsing args: %s", err)
		}
		return toolReadFile(args.Path, args.StartLine, args.EndLine)

	case "read_diff":
		var args struct {
			Path string `json:"path"`
		}
		if err := json.Unmarshal([]byte(tc.Function.Arguments), &args); err != nil {
			return fmt.Sprintf("error parsing args: %s", err)
		}
		return a.toolReadDiff(args.Path)

	case "search_code":
		var args struct {
			Pattern string `json:"pattern"`
			Path    string `json:"path"`
		}
		if err := json.Unmarshal([]byte(tc.Function.Arguments), &args); err != nil {
			return fmt.Sprintf("error parsing args: %s", err)
		}
		return toolSearch(args.Pattern, args.Path)

	case "read_function":
		var args struct {
			Path string `json:"path"`
			Line int    `json:"line"`
		}
		if err := json.Unmarshal([]byte(tc.Function.Arguments), &args); err != nil {
			return fmt.Sprintf("error parsing args: %s", err)
		}
		return a.toolReadFunction(args.Path, args.Line)

	case "find_references":
		var args struct {
			Path string `json:"path"`
			Line int    `json:"line"`
		}
		if err := json.Unmarshal([]byte(tc.Function.Arguments), &args); err != nil {
			return fmt.Sprintf("error parsing args: %s", err)
		}
		return a.toolFindReferences(args.Path, args.Line)

	case "find_definition":
		var args struct {
			Path string `json:"path"`
			Line int    `json:"line"`
		}
		if err := json.Unmarshal([]byte(tc.Function.Arguments), &args); err != nil {
			return fmt.Sprintf("error parsing args: %s", err)
		}
		return a.toolFindDefinition(args.Path, args.Line)

	case "symbol_info":
		var args struct {
			Path string `json:"path"`
			Line int    `json:"line"`
		}
		if err := json.Unmarshal([]byte(tc.Function.Arguments), &args); err != nil {
			return fmt.Sprintf("error parsing args: %s", err)
		}
		return a.toolSymbolInfo(args.Path, args.Line)

	default:
		return fmt.Sprintf("unknown tool: %s", tc.Function.Name)
	}
}

func formatToolArgs(name, argsJSON string) string {
	var args struct {
		Path    string `json:"path"`
		Pattern string `json:"pattern"`
		Line    int    `json:"line"`
	}
	json.Unmarshal([]byte(argsJSON), &args)

	switch name {
	case "read_file":
		if args.Line > 0 {
			return fmt.Sprintf("%s:%d", args.Path, args.Line)
		}
		return args.Path
	case "read_diff":
		if args.Path == "" {
			return "full diff"
		}
		return args.Path
	case "search_code":
		return args.Pattern
	case "read_function", "find_references", "find_definition", "symbol_info":
		return fmt.Sprintf("%s:%d", args.Path, args.Line)
	}
	return ""
}

/* ─── Tool Implementations ─── */

func toolReadFile(path string, startLine, endLine int) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Sprintf("error reading %q: %s", path, err)
	}
	lines := strings.Split(string(data), "\n")
	totalLines := len(lines)

	if startLine <= 0 {
		startLine = 1
	}
	if endLine <= 0 || endLine > totalLines {
		endLine = totalLines
	}
	if endLine < startLine {
		endLine = startLine
	}
	if startLine > totalLines {
		return fmt.Sprintf("file %q has %d lines, start_line %d out of range", path, totalLines, startLine)
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("file: %s (lines %d-%d of %d)\n", path, startLine, endLine, totalLines))
	for i := startLine - 1; i < endLine && i < totalLines; i++ {
		sb.WriteString(fmt.Sprintf("%d: %s\n", i+1, lines[i]))
	}
	return sb.String()
}

func (a *ReviewAgent) toolReadDiff(path string) string {
	if path != "" {
		untracked := GetUntrackedFiles()
		for _, f := range untracked {
			if f == path {
				return fmt.Sprintf("error: %q is an untracked file (new and not yet tracked by git). Use read_file to view its content.", path)
			}
		}
	}
	args := a.diffArgs
	if path != "" {
		args = append(args, "--", path)
	}
	result, err := GetDiff(args)
	if err != nil {
		return fmt.Sprintf("error: %s", err)
	}
	return fmt.Sprintf("file: %s\n\n```diff\n%s\n```\n", pathOrFull(path), result.Diff)
}

func pathOrFull(path string) string {
	if path == "" {
		return "(full diff)"
	}
	return path
}

func toolSearch(pattern, path string) string {
	args := []string{"grep", "-n", "--no-color"}
	if path != "" {
		args = append(args, "--", path)
	}
	args = append(args, "-e", pattern)
	cmd := exec.Command("git", args...)
	out, err := cmd.Output()
	if err != nil {
		return "no matches found"
	}
	result := strings.TrimSpace(string(out))
	if result == "" {
		return "no matches found"
	}
	return result
}

const functionContextLines = 80

// toolReadFunction reads the enclosing function body when a SCIP index is
// available, falling back to a ±30 line window around the change.
func (a *ReviewAgent) toolReadFunction(path string, line int) string {
	if a.indexSet != nil {
		if ix, ok := a.indexSet.IndexFor(path); ok {
			if start := ix.EnclosingDefLine(path, line); start > 0 {
				end := start + functionContextLines
				return toolReadFile(path, start, end)
			}
		}
	}
	return toolReadFunctionFallback(path, line)
}

func toolReadFunctionFallback(path string, line int) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Sprintf("error reading %q: %s", path, err)
	}
	lines := strings.Split(string(data), "\n")
	totalLines := len(lines)

	contextLines := 30
	start := line - 1 - contextLines
	if start < 0 {
		start = 0
	}
	end := line - 1 + contextLines
	if end > totalLines {
		end = totalLines
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("file: %s (lines %d-%d of %d)\n", path, start+1, end, totalLines))
	for i := start; i < end; i++ {
		sb.WriteString(fmt.Sprintf("%d: %s\n", i+1, lines[i]))
	}
	return sb.String()
}

/* ─── SCIP Semantic Tools ─── */

func (a *ReviewAgent) toolFindReferences(path string, line int) string {
	if a.indexSet == nil {
		return "error: semantic index is not available. Use search_code instead."
	}
	locs, err := a.indexSet.FindReferences(path, line)
	if err != nil {
		return fmt.Sprintf("error: %s", err)
	}
	if len(locs) == 0 {
		return fmt.Sprintf("no references found for the symbol at %s:%d", path, line)
	}
	return formatLocations("references", path, line, locs)
}

func (a *ReviewAgent) toolFindDefinition(path string, line int) string {
	if a.indexSet == nil {
		return "error: semantic index is not available. Use search_code instead."
	}
	locs, err := a.indexSet.FindDefinition(path, line)
	if err != nil {
		return fmt.Sprintf("error: %s", err)
	}
	if len(locs) == 0 {
		return fmt.Sprintf("no definition found for the symbol at %s:%d", path, line)
	}
	return formatLocations("definitions", path, line, locs)
}

func (a *ReviewAgent) toolSymbolInfo(path string, line int) string {
	if a.indexSet == nil {
		return "error: semantic index is not available."
	}
	info, err := a.indexSet.SymbolInfoAt(path, line)
	if err != nil {
		return fmt.Sprintf("error: %s", err)
	}
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("symbol: %s\n", info.Symbol))
	if info.DisplayName != "" {
		sb.WriteString(fmt.Sprintf("name: %s\n", info.DisplayName))
	}
	if info.Kind != "" {
		sb.WriteString(fmt.Sprintf("kind: %s\n", info.Kind))
	}
	if info.Signature != "" {
		sb.WriteString(fmt.Sprintf("signature: %s\n", info.Signature))
	}
	if info.Documentation != "" {
		sb.WriteString("documentation:\n")
		sb.WriteString(info.Documentation)
	}
	return sb.String()
}

func formatLocations(kind, path string, line int, locs []scip.Location) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("%s for symbol at %s:%d (%d locations):\n", kind, path, line, len(locs)))
	for _, loc := range locs {
		role := "usage"
		if loc.IsDef {
			role = "definition"
		}
		fmt.Fprintf(&sb, "  %s:%d:%d [%s] %s\n", loc.Path, loc.Line, loc.Character, role, loc.Symbol)
	}
	return sb.String()
}

func truncateToolResult(s string) string {
	runes := []rune(s)
	if len(runes) <= 30000 {
		return s
	}
	return string(runes[:30000]) + "\n...(tool result truncated at 30000 chars)"
}
