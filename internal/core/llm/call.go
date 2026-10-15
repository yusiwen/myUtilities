package llm

import (
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/yusiwen/myUtilities/internal/core/openai"
	"github.com/yusiwen/myUtilities/internal/core/term"
)

// DefaultFastDial is the dial timeout used for every provider that still has a
// fallback behind it: an endpoint that cannot even complete the TCP/TLS
// handshake is abandoned quickly instead of consuming the full request timeout.
const DefaultFastDial = 2 * time.Second

// CallOptions carries the per-invocation overrides and the output preferences
// shared by every module that talks to an LLM. It is the single place where flag
// precedence, the API-key check and the fallback dial policy are decided.
type CallOptions struct {
	FlagBaseURL string
	FlagAPIKey  string
	FlagModel   string

	// Verbose dumps prompts, responses and the raw HTTP payloads.
	Verbose bool

	// Writer receives the progress and verbose output. Nil means os.Stderr.
	Writer io.Writer

	// FastDial overrides DefaultFastDial for providers with a fallback behind
	// them. Zero means DefaultFastDial.
	FastDial time.Duration
}

func (o CallOptions) writer() io.Writer {
	if o.Writer != nil {
		return o.Writer
	}
	return os.Stderr
}

func (o CallOptions) fastDial() time.Duration {
	if o.FastDial > 0 {
		return o.FastDial
	}
	return DefaultFastDial
}

const apiKeyHint = `API key is required. Set it via:
  - OPENAI_API_KEY environment variable
  - --api-key flag
  - ~/.config/mu/llm-config.json (mu set llm provider add --name <name> --base-url <url> --api-key <key>)`

// NewClient builds the HTTP client for one provider, applying the flag overrides
// and failing when the resulting API key is empty. hasFallback selects the short
// dial timeout, so the last provider in a chain — the one with nothing left to
// fall back to — keeps the full request timeout.
func (o CallOptions) NewClient(p *Provider, hasFallback bool) (*openai.Client, error) {
	baseURL, apiKey, model := EffectiveClientConfig(p, o.FlagBaseURL, o.FlagAPIKey, o.FlagModel)
	if baseURL == "" {
		return nil, errors.New("no base_url configured (define it in the shared catalog, or pass --base-url)")
	}
	if apiKey == "" {
		return nil, errors.New(apiKeyHint)
	}

	var client *openai.Client
	if hasFallback {
		client = openai.NewClientWithFastDial(baseURL, apiKey, model, o.fastDial())
	} else {
		client = openai.NewClient(baseURL, apiKey, model)
	}
	if o.Verbose {
		client.DebugWriter = o.writer()
	}
	return client, nil
}

// ForEachProvider walks a provider chain and hands the first provider that
// answers to fn. A provider is skipped when it cannot be resolved or cannot
// produce a client (a missing API key, for instance); a failure from fn is
// retried with the next provider unless that was the last one. Every failure is
// reported as "all provider(s) failed: <last cause>" so the whole repo phrases a
// chain failure identically.
func ForEachProvider(cat *Catalog, names MultiProvider, o CallOptions, fn func(*openai.Client, *Provider) error) error {
	chain := names.Names()
	if len(chain) == 0 {
		return errors.New("no provider configured for this module (add one with 'mu set llm provider add' and reference it in the module config)")
	}

	var lastErr error
	for i, name := range chain {
		hasFallback := i < len(chain)-1

		p, err := cat.Resolve(name)
		if err != nil {
			lastErr = err
			continue
		}

		client, err := o.NewClient(p, hasFallback)
		if err != nil {
			lastErr = fmt.Errorf("provider %q: %w", name, err)
			if hasFallback {
				continue
			}
			break
		}

		fmt.Fprintf(o.writer(), "%s%s%s\n", term.Faint("Calling provider "), term.Bright(p.Name), term.Faint("..."))

		if err := fn(client, p); err != nil {
			lastErr = fmt.Errorf("provider %q failed: %w", name, err)
			if hasFallback {
				continue
			}
			break
		}
		return nil
	}

	if lastErr == nil {
		lastErr = errors.New("no provider could be used")
	}
	return fmt.Errorf("all provider(s) failed: %w", lastErr)
}

// Complete sends one system+user prompt through the first provider that answers
// and returns that provider's result. It is the one-shot path used by ask and
// git commit; the review agent uses ForEachProvider directly because it needs the
// client rather than a single prompt.
func Complete(cat *Catalog, names MultiProvider, o CallOptions, systemPrompt, userPrompt string) (*openai.ChatResult, error) {
	if o.Verbose {
		w := o.writer()
		fmt.Fprintln(w, "─── System Prompt ───")
		fmt.Fprintln(w, systemPrompt)
		fmt.Fprintln(w, "─── User Prompt ───")
		fmt.Fprintln(w, userPrompt)
	}

	var result *openai.ChatResult
	err := ForEachProvider(cat, names, o, func(client *openai.Client, _ *Provider) error {
		start := time.Now()
		res, err := client.ChatCompletion(systemPrompt, userPrompt)
		if err != nil {
			return fmt.Errorf("request failed: %w", err)
		}
		if o.Verbose {
			fmt.Fprintf(o.writer(), "─── Raw Response ───\n%s\n─── API Time: %s ───\n", res.Content, time.Since(start))
		}
		result = res
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}
