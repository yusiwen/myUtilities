package ask

import (
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/morikuni/aec"
	"github.com/yusiwen/myUtilities/core/llm"
	"github.com/yusiwen/myUtilities/core/openai"
)

const systemPrompt = `You are a concise Q&A assistant. Follow these rules strictly:

1. Answer the user's question directly in 2-4 sentences, be concise and precise.
2. At the end of your answer, provide exactly 2-3 reference URLs for further reading.
3. Format URLs as a Markdown list: - [Title](url)
4. Only include real, working URLs from authoritative sources.
5. If you cannot provide proper reference URLs, say "No further reading available."`

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

type Options struct {
	Model    string `help:"Model name to use." short:"m" env:"OPENAI_MODEL"`
	APIKey   string `help:"API key for the AI service." short:"k" env:"OPENAI_API_KEY"`
	BaseURL  string `help:"Base URL of the AI service." short:"u" env:"OPENAI_BASE_URL"`
	Lang     string `help:"Language for the answer." short:"L" default:"en" enum:"en,cn"`
	Verbose  bool   `help:"Print prompts and raw API responses for debugging."`
	Question string `arg:"" name:"question" help:"Question to ask." optional:""`
}

func buildPrompt(lang string) string {
	switch lang {
	case "cn":
		return systemPrompt + "\n\nLanguage: Answer the question and provide URLs in Chinese (Simplified Chinese)."
	default:
		return systemPrompt + "\n\nLanguage: Answer in English."
	}
}

func (o *Options) Run() error {
	cfg, err := llm.LoadConfig("ask")
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
			"  - ~/.config/mu/ask.json config file")
	}

	question := o.Question
	if question == "" {
		stdin, err := io.ReadAll(os.Stdin)
		if err != nil {
			return fmt.Errorf("failed to read stdin: %w", err)
		}
		question = strings.TrimSpace(string(stdin))
	}
	if question == "" {
		return fmt.Errorf("no question provided. Usage: mu ask <question> or pipe input")
	}

	fmt.Fprintf(os.Stderr, "%s\n", faint("Asking..."))

	client := openai.NewClient(cfg.BaseURL, cfg.APIKey, cfg.Model)

	sysPrompt := buildPrompt(o.Lang)
	userPrompt := question

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
	}

	fmt.Println(result.Content)
	fmt.Println(faint(fmt.Sprintf("Tokens: %d prompt + %d completion = %d total (%s)",
		result.PromptTokens, result.CompletionTokens, result.TotalTokens, elapsed)))

	return nil
}
