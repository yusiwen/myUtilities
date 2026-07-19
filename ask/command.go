package ask

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/morikuni/aec"
	"github.com/yusiwen/myUtilities/core/llm"
	"github.com/yusiwen/myUtilities/core/openai"
	"github.com/yusiwen/myUtilities/core/search"
)

const systemPrompt = `You are a concise Q&A assistant. Follow these rules strictly:

1. Answer the user's question directly in 2-4 sentences, be concise and precise.
2. At the end of your answer, provide exactly 2-3 reference URLs for further reading.
3. Format URLs as a Markdown list: - [Title](url)
4. Only include real, working URLs from authoritative sources.
5. If you cannot provide proper reference URLs, say "No further reading available."`

const searchSystemPrompt = `You are a concise Q&A assistant with web search capability. Follow these rules strictly:

1. Answer the user's question based on the provided web search results.
2. Cite sources using [1], [2] etc. corresponding to the numbered search results.
3. After the answer, list the references in the format: - [Title](url)
4. Be concise: 2-4 sentences for the main answer.
5. If the search results are insufficient or irrelevant, say so clearly.
6. Always include the reference URLs from the search results.`

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
	Model        string `help:"Model name to use." short:"m" env:"OPENAI_MODEL"`
	APIKey       string `help:"API key for the AI service." short:"k" env:"OPENAI_API_KEY"`
	BaseURL      string `help:"Base URL of the AI service." short:"u" env:"OPENAI_BASE_URL"`
	Lang         string `help:"Language for the answer." short:"L" default:"en" enum:"en,cn"`
	Search       bool   `help:"Enable web search via Brave Search API." short:"s"`
	SearchAPIKey string `help:"Brave Search API key." env:"BRAVE_SEARCH_API_KEY"`
	Verbose      bool   `help:"Print prompts and raw API responses for debugging."`
	Question     string `arg:"" name:"question" help:"Question to ask." optional:""`
}

func buildSystemPrompt(lang string, withSearch bool) string {
	base := systemPrompt
	if withSearch {
		base = searchSystemPrompt
	}
	switch lang {
	case "cn":
		return base + "\n\nLanguage: Answer the question and provide URLs in Chinese (Simplified Chinese)."
	default:
		return base + "\n\nLanguage: Answer in English."
	}
}

func formatSearchResults(results []search.Result) string {
	if len(results) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("Web search results:\n\n")
	for i, r := range results {
		b.WriteString(fmt.Sprintf("[%d] %s\n    URL: %s\n    %s\n\n", i+1, r.Title, r.URL, r.Snippet))
	}
	return b.String()
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
	if o.SearchAPIKey != "" {
		cfg.SearchAPIKey = o.SearchAPIKey
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

	ctx := context.Background()
	userPrompt := question
	withSearch := o.Search

	if withSearch {
		if cfg.SearchAPIKey == "" {
			return fmt.Errorf("Brave Search API key is required for --search. Set it via:\n" +
				"  - BRAVE_SEARCH_API_KEY environment variable\n" +
				"  - --search-api-key flag\n" +
				"  - search_api_key in ~/.config/mu/ask.json")
		}

		fmt.Fprintf(os.Stderr, "%s\n", faint("Searching web..."))
		searcher := search.NewBraveSearch(cfg.SearchAPIKey)
		results, err := searcher.Search(ctx, question, 5)
		if err != nil {
			return fmt.Errorf("web search failed: %w", err)
		}

		if o.Verbose {
			fmt.Fprintf(os.Stderr, "─── Search Results (%d) ───\n", len(results))
			for _, r := range results {
				fmt.Fprintf(os.Stderr, "  - %s (%s)\n", r.Title, r.URL)
			}
		}

		searchContext := formatSearchResults(results)
		if searchContext != "" {
			userPrompt = searchContext + "Question: " + question
		}
	}

	fmt.Fprintf(os.Stderr, "%s\n", faint("Asking..."))

	client := openai.NewClient(cfg.BaseURL, cfg.APIKey, cfg.Model)

	sysPrompt := buildSystemPrompt(o.Lang, withSearch)

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

	fmt.Fprintln(os.Stderr, faint("Answer:"))
	fmt.Println(result.Content)

	fmt.Fprintln(os.Stderr, faint(fmt.Sprintf("Tokens: %d prompt + %d completion = %d total (%s)",
		result.PromptTokens, result.CompletionTokens, result.TotalTokens, elapsed)))

	return nil
}
