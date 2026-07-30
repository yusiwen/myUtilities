package git

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	coregit "github.com/yusiwen/myUtilities/core/git"
	"github.com/yusiwen/myUtilities/core/openai"
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
## 变更概述
## 文件级分析
## 关注点
## 值得肯定的方面

Be specific — reference code snippets with line numbers when discussing issues.`

type reviewAgent struct {
	client         *openai.Client
	messages       []openai.Message
	tools          []openai.ToolDef
	maxTurns       int
	verbose        bool
	progressWriter io.Writer
	diffArgs       []string
}

func newReviewAgent(client *openai.Client, diff *coregit.DiffResult, diffArgs []string, lang, context, repoName, branchName, commitHash string, maxTurns int, verbose bool) (*reviewAgent, error) {
	nameStatus, _ := coregit.GetNameStatus(diffArgs)

	var initMsg strings.Builder
	initMsg.WriteString(fmt.Sprintf("## Changes Overview\n\nProject: %s\nBranch: %s\nCommit: %s\n\n", repoName, branchName, commitHash))
	initMsg.WriteString("### Diff Stat\n\n```\n")
	initMsg.WriteString(stripANSI(diff.Stat))
	initMsg.WriteString("\n```\n\n")

	if nameStatus != "" {
		initMsg.WriteString("### Changed Files\n\n```\n")
		initMsg.WriteString(nameStatus)
		initMsg.WriteString("\n```\n\n")
	}

	untracked := coregit.GetUntrackedFiles()
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

	if context != "" {
		initMsg.WriteString("### Additional Context\n\n")
		initMsg.WriteString(context)
		initMsg.WriteString("\n\n")
	}

	initMsg.WriteString("Review the changes above. Use the available tools to read diffs and examine files, then produce your final review.")

	sysPrompt := reviewAgentPrompt
	switch lang {
	case "cn":
		sysPrompt += "\n\nLanguage: Write the review in Chinese (Simplified Chinese)."
	default:
		sysPrompt += "\n\nLanguage: Write the review in English."
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

	return &reviewAgent{
		client:         client,
		messages:       messages,
		tools:          tools,
		maxTurns:       maxTurns,
		verbose:        verbose,
		progressWriter: os.Stderr,
		diffArgs:       diffArgs,
	}, nil
}

func (a *reviewAgent) progressf(format string, args ...any) {
	msg := fmt.Sprintf("[agent] "+format, args...)
	fmt.Fprintln(a.progressWriter, faint(msg))
}

func (a *reviewAgent) run() (string, error) {
	statLine := coregit.PlainDiffStat(a.diffArgs)
	if statLine == "" {
		statLine = "no changes"
	}
	a.progressf("Reviewing diff: %s", statLine)

	for turn := 1; turn <= a.maxTurns; turn++ {
		resp, err := a.client.ChatWithTools(a.messages, a.tools)
		if err != nil {
			return "", fmt.Errorf("agent error at turn %d: %w", turn, err)
		}

		if len(resp.ToolCalls) == 0 {
			a.progressf("%d/%d Producing final review (%d chars)", turn, a.maxTurns, len(resp.Content))
			a.progressf("Complete: %d rounds", turn)
			return resp.Content, nil
		}

		asstMsg := openai.Message{
			Role:      "assistant",
			Content:   resp.Content,
			ToolCalls: resp.ToolCalls,
		}
		a.messages = append(a.messages, asstMsg)

		for _, tc := range resp.ToolCalls {
			args := formatToolArgs(tc.Function.Name, tc.Function.Arguments)
			a.progressf("%d/%d %s(%s)", turn, a.maxTurns, tc.Function.Name, args)
			result := a.executeTool(tc)
			a.messages = append(a.messages, openai.Message{
				Role:       "tool",
				ToolCallID: tc.ID,
				Content:    truncateToolResult(result),
			})
		}
	}
	// Max turns reached — force finalize
	a.messages = append(a.messages, openai.Message{
		Role:    "system",
		Content: "You have reached the maximum number of tool calls. Produce your final review now using only the information you have gathered so far. Do not make additional tool calls.",
	})

	resp, err := a.client.ChatWithTools(a.messages, nil)
	if err != nil {
		return "", err
	}

	if resp.Content == "" {
		return "\n\n> ⚠ Review reached maximum turns. Some details may be incomplete.\n", nil
	}

	return resp.Content + "\n\n> ⚠ Review reached maximum turns (" + fmt.Sprintf("%d", a.maxTurns) + "). Some details may be incomplete.\n", nil
}

func (a *reviewAgent) executeTool(tc openai.ToolCall) string {
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
		return toolReadFunction(args.Path, args.Line)

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
	case "read_function":
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

func (a *reviewAgent) toolReadDiff(path string) string {
	if path != "" {
		untracked := coregit.GetUntrackedFiles()
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
	result, err := coregit.GetDiff(args)
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
		return "" // git grep exits 1 when no match
	}
	return strings.TrimSpace(string(out))
}

func toolReadFunction(path string, line int) string {
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

func truncateToolResult(s string) string {
	runes := []rune(s)
	if len(runes) <= 30000 {
		return s
	}
	return string(runes[:30000]) + "\n...(tool result truncated at 30000 chars)"
}
