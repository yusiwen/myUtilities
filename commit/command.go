package commit

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/yusiwen/myUtilities/core/git"
	"github.com/yusiwen/myUtilities/core/openai"
)

const systemPrompt = `You are a commit message generator. Generate a conventional commit message from the git diff provided.

Rules:
- First line: <type>(<scope>): <description>
- Type must be one of: feat, fix, docs, style, refactor, perf, test, build, ci, chore, revert
- Keep the description under 72 characters
- After the first line, leave a blank line, then add a brief bullet-point summary of key changes
- Each bullet point should be concise, under 80 characters
- If the diff involves many files, use a higher-level summary
- Do not wrap the message in quotes, backticks, or code blocks`

type Options struct {
	Model   string `help:"Model name to use." short:"m" env:"OPENAI_MODEL"`
	APIKey  string `help:"API key for the AI service." short:"k" env:"OPENAI_API_KEY"`
	BaseURL string `help:"Base URL of the AI service." short:"u" env:"OPENAI_BASE_URL"`
	DryRun  bool   `help:"Print the generated message without committing." short:"n"`
	Yes     bool   `help:"Skip confirmation and commit directly." short:"y"`
}

func (o *Options) Run() error {
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

	if err := git.CheckPreflight(); err != nil {
		return err
	}

	diff, err := git.GetStagedDiff()
	if err != nil {
		return err
	}

	fmt.Fprintf(os.Stderr, "Generating commit message from diff (%d chars)...\n", len([]rune(diff.Diff)))

	client := openai.NewClient(cfg.BaseURL, cfg.APIKey, cfg.Model)

	userPrompt := "Generate a conventional commit message for this git diff:\n\n```diff\n" + diff.Diff + "\n```"
	msg, err := client.ChatCompletion(systemPrompt, userPrompt)
	if err != nil {
		return err
	}

	sep := strings.Repeat("─", 50)
	fmt.Printf("\n%s\n%s\n%s\n\n%s\n", sep, msg, sep, diff.Stat)

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

func confirmAndEdit(msg string) (string, error) {
	reader := bufio.NewReader(os.Stdin)

	for {
		fmt.Print("Commit with this message? (y)es / (e)dit / (n)o: ")
		response, err := reader.ReadString('\n')
		if err != nil {
			return "", fmt.Errorf("failed to read input: %w", err)
		}
		response = strings.ToLower(strings.TrimSpace(response))

		switch {
		case response == "n" || response == "no":
			fmt.Println("Aborted.")
			return "", nil
		case response == "e" || response == "edit":
			edited, err := openEditor(msg)
			if err != nil {
				return "", err
			}
			edited = strings.TrimSpace(edited)
			if edited == "" {
				fmt.Println("Aborted: empty message.")
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
			fmt.Println("Invalid response.")
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
