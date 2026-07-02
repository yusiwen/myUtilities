package git

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"

	"github.com/morikuni/aec"
	coregit "github.com/yusiwen/myUtilities/core/git"
	"github.com/yusiwen/myUtilities/core/openai"
)

const baseSystemPrompt = `You are a commit message generator. Generate a conventional commit message from the git diff provided.

Rules:
- First line: <type>(<scope>): <description>
- Type must be one of: feat, fix, docs, style, refactor, perf, test, build, ci, chore, revert
- Keep the description under 72 characters
- After the first line, leave a blank line, then add a brief bullet-point summary of key changes
- Each bullet point should be concise, under 80 characters
- If the diff involves many files, use a higher-level summary
- Do not wrap the message in quotes, backticks, or code blocks`

var noColor bool

func init() {
	if os.Getenv("NO_COLOR") != "" {
		noColor = true
	}
}

func faint(s string) string {
	if noColor {
		return s
	}
	return aec.Apply(s, aec.Faint)
}

func bright(s string) string {
	if noColor {
		return s
	}
	return aec.Apply(s, aec.WhiteF)
}

var ansiRE = regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]`)

func stripANSI(s string) string {
	return ansiRE.ReplaceAllString(s, "")
}

func buildSystemPrompt(lang string) string {
	prompt := baseSystemPrompt
	switch lang {
	case "cn":
		prompt += "\n\nLanguage: Write the commit message description and bullet points in Chinese (Simplified Chinese). Keep the conventional commit type prefix (feat:, fix:, etc.) in English."
	default:
		prompt += "\n\nLanguage: Write the commit message in English."
	}
	return prompt
}

type CommitOptions struct {
	Model        string `help:"Model name to use." short:"m" env:"OPENAI_MODEL"`
	APIKey       string `help:"API key for the AI service." short:"k" env:"OPENAI_API_KEY"`
	BaseURL      string `help:"Base URL of the AI service." short:"u" env:"OPENAI_BASE_URL"`
	DryRun       bool   `help:"Print the generated message without committing." short:"n"`
	Yes          bool   `help:"Skip confirmation and commit directly." short:"y"`
	Verbose      bool   `help:"Print prompts and raw API responses for debugging."`
	DiffStrategy string `help:"How much diff to send to AI." short:"s" default:"auto" enum:"auto,full,summary"`
	Lang         string `help:"Language for commit message." short:"L" default:"en" enum:"en,cn"`
}

func (o *CommitOptions) Run() error {
	cfg, err := loadConfig()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	if o.Model != "" {
		cfg.Model = o.Model
	}
	if o.APIKey != "" {
		cfg.APIKey = o.APIKey
	}
	if o.BaseURL != "" {
		cfg.BaseURL = o.BaseURL
	}

	if cfg.APIKey == "" {
		return fmt.Errorf("API key is required. Set it via:\n" +
			"  - OPENAI_API_KEY environment variable\n" +
			"  - --api-key flag\n" +
			"  - ~/.config/mu/commit.json config file")
	}

	if err := coregit.CheckPreflight(); err != nil {
		return err
	}

	diff, err := coregit.GetStagedDiff()
	if err != nil {
		return err
	}

	strategy := resolveStrategy(o.DiffStrategy, diff.RawLen)

	var nameStatus string
	if strategy == "summary" || (strategy == "auto" && diff.RawLen > 16000) {
		nameStatus, err = coregit.GetStagedNameStatus()
		if err != nil {
			return err
		}
	}

	fmt.Fprintf(os.Stderr, "%s%s%s\n",
		faint("Generating commit message from diff ("),
		bright(fmt.Sprintf("%d", diff.RawLen)),
		faint(fmt.Sprintf(" chars, strategy: %s)...", strategy)))

	client := openai.NewClient(cfg.BaseURL, cfg.APIKey, cfg.Model)
	client.DebugWriter = os.Stderr

	sysPrompt := buildSystemPrompt(o.Lang)
	userPrompt := buildUserPrompt(strategy, diff.Diff, diff.Stat, nameStatus)

	if o.Verbose {
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
	}

	sep := strings.Repeat("─", 50)
	fmt.Printf("\n%s\n%s\n%s\n\n%s\n", sep, result.Content, sep, diff.Stat)
	fmt.Println(bright(fmt.Sprintf("Tokens: %d prompt + %d completion = %d total",
		result.PromptTokens, result.CompletionTokens, result.TotalTokens)))

	msg := result.Content

	if o.DryRun {
		return nil
	}

	if !o.Yes {
		msg, err = confirmAndEdit(msg)
		if err != nil {
			return err
		}
		if msg == "" {
			return nil
		}
	}

	commitCmd := exec.Command("git", "commit", "-m", msg)
	commitCmd.Stdout = os.Stdout
	commitCmd.Stderr = os.Stderr
	if err := commitCmd.Run(); err != nil {
		return fmt.Errorf("git commit failed: %w", err)
	}

	return nil
}

func resolveStrategy(flag string, diffLen int) string {
	if flag != "auto" {
		return flag
	}
	switch {
	case diffLen <= 6000:
		return "full"
	case diffLen <= 16000:
		return "medium"
	default:
		return "summary"
	}
}

func buildUserPrompt(strategy, diff, stat, nameStatus string) string {
	switch strategy {
	case "medium":
		return "Generate a conventional commit message for this git diff stat:\n\n" + stripANSI(stat) +
			"\nAnd here are the first lines of the diff:\n\n```diff\n" + coregit.Truncate(diff, 3000) + "\n```"
	case "summary":
		summary := "Generate a conventional commit message for this git diff stat:\n\n" + stripANSI(stat)
		if nameStatus != "" {
			summary += "\nChanged files:\n\n" + nameStatus
		}
		return summary
	default: // "full"
		return "Generate a conventional commit message for this git diff:\n\n```diff\n" + diff + "\n```"
	}
}

func confirmAndEdit(msg string) (string, error) {
	reader := bufio.NewReader(os.Stdin)

	for {
		fmt.Print(faint("Commit with this message? (y)es / (e)dit / (n)o: "))
		response, err := reader.ReadString('\n')
		if err != nil {
			return "", fmt.Errorf("failed to read input: %w", err)
		}
		response = strings.ToLower(strings.TrimSpace(response))

		switch {
		case response == "n" || response == "no":
			fmt.Println(faint("Aborted."))
			return "", nil
		case response == "e" || response == "edit":
			edited, err := openEditor(msg)
			if err != nil {
				return "", err
			}
			edited = strings.TrimSpace(edited)
			if edited == "" {
				fmt.Println(faint("Aborted: empty message."))
				return "", nil
			}
			if edited == msg {
				continue
			}
			sep := strings.Repeat("─", 50)
			fmt.Printf("\n%s\n%s\n%s\n", sep, edited, sep)
			msg = edited
		case response == "y" || response == "yes" || response == "":
			return msg, nil
		default:
			fmt.Println(faint("Invalid response."))
		}
	}
}

func openEditor(initialMsg string) (string, error) {
	tmpFile, err := os.CreateTemp("", "mu-commit-msg-*")
	if err != nil {
		return "", fmt.Errorf("failed to create temp file: %w", err)
	}
	defer os.Remove(tmpFile.Name())

	if _, err := tmpFile.WriteString(initialMsg); err != nil {
		tmpFile.Close()
		return "", fmt.Errorf("failed to write temp file: %w", err)
	}
	tmpFile.Close()

	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = os.Getenv("VISUAL")
	}
	if editor == "" {
		for _, candidate := range []string{"vi", "vim", "nano"} {
			if _, err := exec.LookPath(candidate); err == nil {
				editor = candidate
				break
			}
		}
	}
	if editor == "" {
		return "", fmt.Errorf("no editor found (set $EDITOR or install vi/nano)")
	}

	editCmd := exec.Command(editor, tmpFile.Name())
	editCmd.Stdin = os.Stdin
	editCmd.Stdout = os.Stdout
	editCmd.Stderr = os.Stderr
	if err := editCmd.Run(); err != nil {
		return "", fmt.Errorf("editor exited with error: %w", err)
	}

	data, err := os.ReadFile(tmpFile.Name())
	if err != nil {
		return "", fmt.Errorf("failed to read edited message: %w", err)
	}

	return strings.TrimSpace(string(data)), nil
}
