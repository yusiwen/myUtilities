package git

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/charmbracelet/glamour"
	coregit "github.com/yusiwen/myUtilities/core/git"
	"github.com/yusiwen/myUtilities/core/openai"
	"github.com/yusiwen/myUtilities/core/term"
	xterm "golang.org/x/term"
)

const reviewSystemPrompt = `You are a senior software engineer conducting a thorough code review. Analyze the provided git diff and produce a structured markdown code review.

Please include the following sections:

## 变更概述
A brief summary of what changes this diff introduces and the overall purpose.

## 文件级分析
For each changed file, describe:
- What was changed
- Why the change was made (infer from context)
- Potential impact on the codebase

## 关注点
List any of the following that apply:
- Potential bugs or regressions
- Security concerns
- Performance issues
- Maintainability or readability improvements
- Violations of best practices or project conventions

## 值得肯定的方面
- Well-structured changes
- Good naming, proper error handling, effective patterns

Write in a constructive, professional tone. Be specific — reference code snippets when discussing issues.`

func (o *ReviewOptions) Run() error {
	if o.List {
		cmd := &ReviewListCmd{All: o.ListAll}
		return cmd.Run()
	}

	gc, err := coregit.LoadGitConfig()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	moduleCfg, err := coregit.GetModuleConfig(gc, "review")
	if err != nil {
		return err
	}

	provider, err := coregit.ResolveProvider(gc, moduleCfg.Provider)
	if err != nil {
		return err
	}

	baseURL := provider.BaseURL
	apiKey := provider.APIKey
	model := provider.Model
	if o.BaseURL != "" {
		baseURL = o.BaseURL
	}
	if o.APIKey != "" {
		apiKey = o.APIKey
	}
	if o.Model != "" {
		model = o.Model
	}

	if apiKey == "" {
		return fmt.Errorf("API key is required. Set it via:\n" +
			"  - OPENAI_API_KEY environment variable\n" +
			"  - --api-key flag\n" +
			"  - 'mu set git provider add --name <name> --api-key <key> ...'")
	}

	if err := coregit.CheckPreflight(); err != nil {
		return err
	}

	var diffArgs []string
	if o.Staged {
		diffArgs = append(diffArgs, "--staged")
	}
	if o.Base != "" {
		target := o.Target
		if target == "" {
			target = "HEAD"
		}
		diffArgs = append(diffArgs, o.Base+".."+target)
	} else if o.Target != "" {
		return fmt.Errorf("--target requires --base")
	}
	if len(o.Paths) > 0 {
		diffArgs = append(diffArgs, "--")
		diffArgs = append(diffArgs, o.Paths...)
	}

	diff, err := coregit.GetDiff(diffArgs)
	if err != nil {
		return err
	}

	lang := o.Lang
	if lang == "" {
		lang = moduleCfg.Lang
	}

	repoName := coregit.RepoName()
	branchName := coregit.FileSafeName(coregit.CurrentBranch())
	commitHash := coregit.ShortCommit()

	sysPrompt := buildReviewSystemPrompt(lang)

	if o.Verbose {
		fmt.Fprintln(os.Stderr, "─── Review Args ───")
		fmt.Fprintln(os.Stderr, "git diff", strings.Join(diffArgs, " "))
	}

	client := openai.NewClient(baseURL, apiKey, model)

	if o.Verbose {
		client.DebugWriter = os.Stderr
		fmt.Fprintln(os.Stderr, sysPrompt)
	}

	maxTurns := o.MaxTurns
	start := time.Now()
	agent, err := coregit.NewReviewAgent(client, diff, diffArgs, lang, o.Context, repoName, branchName, commitHash, maxTurns, o.Verbose)
	if err != nil {
		return err
	}

	agentResult, err := agent.Run()
	elapsed := time.Since(start)

	if err != nil {
		return err
	}

	if o.Verbose {
		fmt.Fprintf(os.Stderr, "─── Agent completed in %s ───\n", elapsed)
	}

	timestamp := time.Now()
	base := o.Base
	target := o.Target
	if target == "" {
		target = "HEAD"
	}

	frontMatter := buildFrontMatter(repoName, branchName, commitHash, base, target, o.Staged, o.Paths, lang, "agent", diff, o.Context, timestamp, diffArgs)
	saveContent := frontMatter + "\n" + agentResult.Content

	reviewsDir := moduleCfg.ReviewsDirPath()
	if err := os.MkdirAll(reviewsDir, 0700); err != nil {
		return fmt.Errorf("failed to create reviews directory: %w", err)
	}

	fileName := fmt.Sprintf("%s_%s_%s.md",
		coregit.FileSafeName(repoName),
		branchName,
		timestamp.Format("20060102-150405"))
	savePath := filepath.Join(reviewsDir, fileName)

	if err := os.WriteFile(savePath, []byte(saveContent), 0644); err != nil {
		return fmt.Errorf("failed to save review: %w", err)
	}

	fmt.Fprintf(os.Stderr, "%s%s%s\n",
		term.Faint("Review saved to "),
		term.Bright(savePath),
		term.Faint(""))

	out := agentResult.Content
	if xterm.IsTerminal(int(os.Stdout.Fd())) {
		r, err := glamour.NewTermRenderer(glamour.WithAutoStyle())
		if err == nil {
			rendered, err := r.Render(out)
			if err == nil {
				out = rendered
			}
		}
	}

	pagedOutput(out)
	return nil
}

func buildFrontMatter(project, branch, commit, base, target string, staged bool, paths []string, lang, strategy string, diff *coregit.DiffResult, context string, timestamp time.Time, diffArgs []string) string {
	var sb strings.Builder
	sb.WriteString("---\n")
	sb.WriteString(fmt.Sprintf("project: %s\n", project))
	sb.WriteString(fmt.Sprintf("branch: %s\n", branch))
	sb.WriteString(fmt.Sprintf("commit: %s\n", commit))

	if staged {
		sb.WriteString("staged: true\n")
	} else {
		sb.WriteString("staged: false\n")
	}
	if base != "" {
		sb.WriteString(fmt.Sprintf("base: %s\n", base))
	}
	if target != "" && target != "HEAD" {
		sb.WriteString(fmt.Sprintf("target: %s\n", target))
	}
	if len(paths) > 0 {
		sb.WriteString(fmt.Sprintf("paths: %s\n", strings.Join(paths, " ")))
	}
	sb.WriteString(fmt.Sprintf("lang: %s\n", lang))
	sb.WriteString(fmt.Sprintf("strategy: %s\n", strategy))
	sb.WriteString(fmt.Sprintf("diff_size: %d\n", diff.RawLen))
	if stat := coregit.PlainDiffStat(diffArgs); stat != "" {
		sb.WriteString(fmt.Sprintf("diff_stat: %s\n", stat))
	}
	if context != "" {
		sb.WriteString(fmt.Sprintf("context: %s\n", context))
	}
	sb.WriteString(fmt.Sprintf("timestamp: %s\n", timestamp.Format(time.RFC3339)))
	sb.WriteString("---")
	return sb.String()
}

/* ─── mu git review list ─── */

func (o *ReviewListCmd) Run() error {
	gc, err := coregit.LoadGitConfig()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	moduleCfg, err := coregit.GetModuleConfig(gc, "review")
	if err != nil {
		return err
	}

	reviewsDir := moduleCfg.ReviewsDirPath()

	entries, err := os.ReadDir(reviewsDir)
	if err != nil {
		if os.IsNotExist(err) {
			fmt.Println("No review reports found.")
			return nil
		}
		return fmt.Errorf("failed to read reviews directory: %w", err)
	}

	var currentProject string
	if !o.All {
		currentProject = coregit.RepoName()
	}

	type reviewEntry struct {
		project   string
		branch    string
		timestamp string
		strategy  string
		filePath  string
	}
	var reviews []reviewEntry

	fmRE := regexp.MustCompile(`(?m)^(\w+):\s*(.*)$`)

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}

		fullPath := filepath.Join(reviewsDir, entry.Name())
		data, err := os.ReadFile(fullPath)
		if err != nil {
			continue
		}

		content := string(data)
		if !strings.HasPrefix(content, "---\n") {
			continue
		}

		end := strings.Index(content[4:], "\n---")
		if end == -1 {
			continue
		}
		fmBlock := content[4 : 4+end]

		fm := make(map[string]string)
		for _, m := range fmRE.FindAllStringSubmatch(fmBlock, -1) {
			fm[m[1]] = m[2]
		}

		project := fm["project"]
		if !o.All && project != currentProject {
			continue
		}

		reviews = append(reviews, reviewEntry{
			project:   project,
			branch:    fm["branch"],
			timestamp: fm["timestamp"],
			strategy:  fm["strategy"],
			filePath:  fullPath,
		})
	}

	if len(reviews) == 0 {
		if o.All {
			fmt.Println("No review reports found.")
		} else {
			fmt.Printf("No review reports found for project %q.\n", currentProject)
		}
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, "Date\tProject\tBranch\tStrategy")
	fmt.Fprintln(w, "----\t-------\t------\t--------")
	for _, r := range reviews {
		t, err := time.Parse(time.RFC3339, r.timestamp)
		dateStr := r.timestamp
		if err == nil {
			dateStr = t.Format("2006-01-02 15:04:05")
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", dateStr, r.project, r.branch, r.strategy)
	}
	w.Flush()
	return nil
}

/* ─── Prompt & Output Helpers ─── */

func buildReviewSystemPrompt(lang string) string {
	switch lang {
	case "cn":
		return reviewSystemPrompt + "\n\nLanguage: Write the review in Chinese (Simplified Chinese). The section headers should remain in Chinese as shown above."
	default:
		return reviewSystemPrompt + "\n\nLanguage: Write the review in English."
	}
}

func buildReviewUserPrompt(strategy string, diff *coregit.DiffResult, userContext string) string {
	var sb strings.Builder

	sb.WriteString("Please review the following code changes.\n\n")

	if userContext != "" {
		sb.WriteString("Additional context from the author:\n")
		sb.WriteString(userContext)
		sb.WriteString("\n\n")
	}

	switch strategy {
	case "summary":
		sb.WriteString("Since the diff is large, review the following summary and stat, focusing on the most impactful changes:\n\n")
		sb.WriteString("```\n")
		sb.WriteString(term.StripANSI(diff.Stat))
		sb.WriteString("\n```")
	case "medium":
		sb.WriteString("Diff stat:\n\n```\n")
		sb.WriteString(term.StripANSI(diff.Stat))
		sb.WriteString("\n```\n\n")
		sb.WriteString("Partial diff (truncated):\n\n```diff\n")
		sb.WriteString(coregit.Truncate(diff.Diff, 3000))
		sb.WriteString("\n```")
	default:
		sb.WriteString("```diff\n")
		sb.WriteString(diff.Diff)
		sb.WriteString("\n```")
	}

	return sb.String()
}

func pagedOutput(content string) {
	if !xterm.IsTerminal(int(os.Stdout.Fd())) {
		fmt.Print(content)
		return
	}

	cmdStr := pagerCommand()
	if cmdStr == "" {
		fmt.Print(content)
		return
	}

	cmd := exec.Command("sh", "-c", cmdStr)
	cmd.Stdin = strings.NewReader(content)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		fmt.Fprint(os.Stderr, content)
	}
}

func pagerCommand() string {
	if p := os.Getenv("PAGER"); p != "" {
		return p
	}

	if _, err := exec.LookPath("less"); err == nil {
		return "less -R"
	}
	if _, err := exec.LookPath("more"); err == nil {
		return "more"
	}
	return ""
}

func resolveReviewStrategy(diffLen int) string {
	switch {
	case diffLen <= 6000:
		return "full"
	case diffLen <= 16000:
		return "medium"
	default:
		return "summary"
	}
}
