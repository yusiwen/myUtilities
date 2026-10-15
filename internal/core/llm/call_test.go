package llm

import (
	"bytes"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/yusiwen/myUtilities/internal/core/openai"
)

// closedPortURL returns a URL whose port was just released, so a dial fails
// immediately with "connection refused" instead of hanging.
func closedPortURL(t *testing.T) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	addr := l.Addr().String()
	if err := l.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	return "http://" + addr
}

func TestCallOptionsFastDialDefaults(t *testing.T) {
	if got := (CallOptions{}).fastDial(); got != DefaultFastDial {
		t.Errorf("default fastDial = %s, want %s", got, DefaultFastDial)
	}
	if got := (CallOptions{FastDial: 50 * time.Millisecond}).fastDial(); got != 50*time.Millisecond {
		t.Errorf("overridden fastDial = %s, want 50ms", got)
	}
}

// The fast-fail policy is observable through the client it produces: a provider
// with a fallback behind it gets a short dial timeout, the last one does not.
// Mutation check: swapping in openai.NewClient for the fallback branch makes the
// first assertion fail (DialTimeout would be 0).
func TestNewClientFastDialPolicy(t *testing.T) {
	p := &Provider{Name: "p", BaseURL: "https://example.invalid/v1", APIKey: "sk-x", Model: "m"}

	first, err := (CallOptions{FastDial: 700 * time.Millisecond}).NewClient(p, true)
	if err != nil {
		t.Fatalf("NewClient(hasFallback): %v", err)
	}
	if got := first.DialTimeout(); got != 700*time.Millisecond {
		t.Errorf("fallback client DialTimeout = %s, want 700ms", got)
	}

	last, err := (CallOptions{FastDial: 700 * time.Millisecond}).NewClient(p, false)
	if err != nil {
		t.Fatalf("NewClient(last): %v", err)
	}
	if got := last.DialTimeout(); got != 0 {
		t.Errorf("last-provider client DialTimeout = %s, want 0 (full request timeout)", got)
	}

	// With no override the shared default applies.
	def, err := (CallOptions{}).NewClient(p, true)
	if err != nil {
		t.Fatalf("NewClient(default): %v", err)
	}
	if got := def.DialTimeout(); got != DefaultFastDial {
		t.Errorf("default fallback DialTimeout = %s, want %s", got, DefaultFastDial)
	}
}

func TestNewClientAppliesFlagsAndChecksKey(t *testing.T) {
	p := &Provider{Name: "p", BaseURL: "https://provider/v1", APIKey: "sk-p", Model: "p-model"}

	c, err := (CallOptions{FlagBaseURL: "https://flag/v1", FlagModel: "flag-model"}).NewClient(p, false)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	if c.BaseURL != "https://flag/v1" || c.Model != "flag-model" || c.APIKey != "sk-p" {
		t.Errorf("client = %+v, want flag base_url/model with the provider api_key", c)
	}

	noKey := &Provider{Name: "p", BaseURL: "https://provider/v1"}
	if _, err := (CallOptions{}).NewClient(noKey, false); err == nil {
		t.Error("NewClient without an api key: expected an error")
	} else if !strings.Contains(err.Error(), "API key is required") {
		t.Errorf("error = %q, want the shared API-key hint", err)
	}

	// A flag-supplied key rescues a provider that has none.
	if _, err := (CallOptions{FlagAPIKey: "sk-flag"}).NewClient(noKey, false); err != nil {
		t.Errorf("NewClient with --api-key override: %v", err)
	}

	// A provider left without an endpoint (for example after the shared entry it
	// inherited from was removed) must say so instead of posting to a bare path.
	noURL := &Provider{Name: "p", APIKey: "sk"}
	if _, err := (CallOptions{}).NewClient(noURL, false); err == nil {
		t.Error("NewClient without a base url: expected an error")
	} else if !strings.Contains(err.Error(), "no base_url") {
		t.Errorf("error = %q, want a missing base_url message", err)
	}
	// --base-url repairs it.
	if _, err := (CallOptions{FlagBaseURL: "https://flag/v1"}).NewClient(noURL, false); err != nil {
		t.Errorf("NewClient with --base-url override: %v", err)
	}
}

func TestForEachProviderWalksTheChain(t *testing.T) {
	cat := &Catalog{Source: "llm-config.json", Providers: []Provider{
		{Name: "first", BaseURL: "https://first/v1", APIKey: "sk"},
		{Name: "second", BaseURL: "https://second/v1", APIKey: "sk"},
	}}

	var out bytes.Buffer
	var seen []string
	err := ForEachProvider(cat, MultiProvider{"first", "second"}, CallOptions{Writer: &out},
		func(_ *openai.Client, p *Provider) error {
			seen = append(seen, p.Name)
			if p.Name == "first" {
				return fmt.Errorf("provider is down")
			}
			return nil
		})
	if err != nil {
		t.Fatalf("ForEachProvider: %v", err)
	}
	if strings.Join(seen, ",") != "first,second" {
		t.Errorf("provider order = %#v, want first,second", seen)
	}
	if !strings.Contains(out.String(), "Calling provider first") || !strings.Contains(out.String(), "Calling provider second") {
		t.Errorf("progress output = %q, want a line per attempted provider", out.String())
	}
}

// A name that is not in the catalog is skipped, and the next provider is used.
func TestForEachProviderSkipsUnresolvableName(t *testing.T) {
	cat := &Catalog{Source: "llm-config.json", Providers: []Provider{
		{Name: "real", BaseURL: "https://real/v1", APIKey: "sk"},
	}}

	var used string
	err := ForEachProvider(cat, MultiProvider{"ghost", "real"}, CallOptions{Writer: &bytes.Buffer{}},
		func(_ *openai.Client, p *Provider) error {
			used = p.Name
			return nil
		})
	if err != nil {
		t.Fatalf("ForEachProvider: %v", err)
	}
	if used != "real" {
		t.Errorf("used provider = %q, want real", used)
	}
}

func TestForEachProviderAllFail(t *testing.T) {
	cat := &Catalog{Source: "llm-config.json", Providers: []Provider{
		{Name: "a", BaseURL: "https://a/v1", APIKey: "sk"},
		{Name: "b", BaseURL: "https://b/v1", APIKey: "sk"},
	}}

	err := ForEachProvider(cat, MultiProvider{"a", "b"}, CallOptions{Writer: &bytes.Buffer{}},
		func(_ *openai.Client, p *Provider) error {
			return fmt.Errorf("%s is down", p.Name)
		})
	if err == nil {
		t.Fatal("expected an error")
	}
	for _, want := range []string{"all provider(s) failed", `provider "b" failed`, "b is down"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error = %q, want it to contain %q", err, want)
		}
	}
}

func TestForEachProviderNoProviders(t *testing.T) {
	err := ForEachProvider(&Catalog{}, nil, CallOptions{Writer: &bytes.Buffer{}},
		func(*openai.Client, *Provider) error { return nil })
	if err == nil {
		t.Fatal("expected an error for an empty chain")
	}
	if !strings.Contains(err.Error(), "no provider configured") {
		t.Errorf("error = %q, want a no-provider hint", err)
	}
}

// An empty API key skips that provider instead of aborting the chain.
func TestForEachProviderSkipsProviderWithoutKey(t *testing.T) {
	cat := &Catalog{Source: "llm-config.json", Providers: []Provider{
		{Name: "nokey", BaseURL: "https://nokey/v1"},
		{Name: "ok", BaseURL: "https://ok/v1", APIKey: "sk"},
	}}

	var used string
	err := ForEachProvider(cat, MultiProvider{"nokey", "ok"}, CallOptions{Writer: &bytes.Buffer{}},
		func(_ *openai.Client, p *Provider) error {
			used = p.Name
			return nil
		})
	if err != nil {
		t.Fatalf("ForEachProvider: %v", err)
	}
	if used != "ok" {
		t.Errorf("used provider = %q, want ok", used)
	}
}

// Complete is the one-shot path: it must send the prompt, parse the answer and
// fall back to the next provider when the first one cannot be dialed.
func TestCompleteFallsBackToSecondProvider(t *testing.T) {
	var gotAuth, gotBody string
	good := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		buf := new(bytes.Buffer)
		if _, err := buf.ReadFrom(r.Body); err != nil {
			t.Errorf("read request body: %v", err)
		}
		gotBody = buf.String()
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"choices":[{"message":{"content":"the answer"}}],"usage":{"prompt_tokens":1,"completion_tokens":2,"total_tokens":3}}`)
	}))
	defer good.Close()

	cat := &Catalog{Source: "llm-config.json", Providers: []Provider{
		{Name: "dead", BaseURL: closedPortURL(t), APIKey: "sk-dead", Model: "m"},
		{Name: "good", BaseURL: good.URL, APIKey: "sk-good", Model: "m"},
	}}

	var out bytes.Buffer
	res, err := Complete(cat, MultiProvider{"dead", "good"}, CallOptions{Writer: &out}, "SYSTEM", "USER")
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if res.Content != "the answer" || res.TotalTokens != 3 {
		t.Errorf("result = %+v, want the good provider's answer", res)
	}
	if gotAuth != "Bearer sk-good" {
		t.Errorf("Authorization = %q, want the second provider's key", gotAuth)
	}
	if !strings.Contains(gotBody, "SYSTEM") || !strings.Contains(gotBody, "USER") {
		t.Errorf("request body = %q, want both prompts", gotBody)
	}
	if !strings.Contains(out.String(), "Calling provider dead") || !strings.Contains(out.String(), "Calling provider good") {
		t.Errorf("progress output = %q, want both attempts", out.String())
	}
}

func TestCompleteVerboseDumpsPromptsAndTiming(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `{"choices":[{"message":{"content":"ok"}}]}`)
	}))
	defer srv.Close()

	cat := &Catalog{Source: "llm-config.json", Providers: []Provider{
		{Name: "p", BaseURL: srv.URL, APIKey: "sk", Model: "m"},
	}}

	var out bytes.Buffer
	if _, err := Complete(cat, MultiProvider{"p"}, CallOptions{Writer: &out, Verbose: true}, "SYSTEM-PROMPT", "USER-PROMPT"); err != nil {
		t.Fatalf("Complete: %v", err)
	}
	for _, want := range []string{"─── System Prompt ───", "SYSTEM-PROMPT", "─── User Prompt ───", "USER-PROMPT", "─── Raw Response ───", "─── API Time:"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("verbose output is missing %q:\n%s", want, out.String())
		}
	}
}
