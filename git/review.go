package git

import (
	"fmt"
	"os"
	"strings"
	"time"

	coregit "github.com/yusiwen/myUtilities/core/git"
	"github.com/yusiwen/myUtilities/core/openai"
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
	gc, err := LoadGitConfig()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	moduleCfg, err := GetModuleConfig(gc, "review")
	if err != nil {
		return err
	}

	provider, err := ResolveProvider(gc, moduleCfg.Provider)
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

	strategy := resolveReviewStrategy(diff.RawLen)

	lang := o.Lang
	if lang == "" {
		lang = moduleCfg.Lang
	}

	sysPrompt := buildReviewSystemPrompt(lang)
	userPrompt := buildReviewUserPrompt(strategy, diff, o.Context)

	if o.Verbose {
		fmt.Fprintln(os.Stderr, "─── Review Args ───")
		fmt.Fprintln(os.Stderr, "git diff", strings.Join(diffArgs, " "))
		fmt.Fprintf(os.Stderr, "Strategy: %s, Diff size: %d chars\n", strategy, diff.RawLen)
	}

	fmt.Fprintf(os.Stderr, "%s%s%s\n",
		faint("Running AI code review (diff: "),
		bright(fmt.Sprintf("%d", diff.RawLen)),
		faint(" chars)..."))

	client := openai.NewClient(baseURL, apiKey, model)

	if o.Verbose {
		client.DebugWriter = os.Stderr
		fmt.Fprintln(os.Stderr, "─── System Prompt ───")
		fmt.Fprintln(os.Stderr, sysPrompt)
		fmt.Fprintln(os.Stderr, "─── User Prompt ───")
		fmt.Fprintln(os.Stderr, userPrompt)
	}

	start := time.Now()
	result, err := client.ChatCompletion(sysPrompt, userPrompt)
	elapsed := time.Since(start)
	if err != nil {
		return err
	}

	if o.Verbose {
		fmt.Fprintf(os.Stderr, "─── Raw Response ───\n%s\n", result.Content)
		fmt.Fprintf(os.Stderr, "─── API Time: %s ───\n", elapsed)
		fmt.Fprintln(os.Stderr, bright(fmt.Sprintf("Tokens: %d prompt + %d completion = %d total",
			result.PromptTokens, result.CompletionTokens, result.TotalTokens)))
	}

	fmt.Println(result.Content)
	return nil
}

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
		sb.WriteString(stripANSI(diff.Stat))
		sb.WriteString("\n```")
	case "medium":
		sb.WriteString("Diff stat:\n\n```\n")
		sb.WriteString(stripANSI(diff.Stat))
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
