package openai

import (
	"testing"
	"time"
)

// Both constructors must apply the same per-endpoint defaults, so a provider used
// with a fallback behind it behaves exactly like the same provider used alone.
func TestConstructorsApplyProviderDefaults(t *testing.T) {
	const deepseek = "https://api.deepseek.com/v1"

	plain := NewClient(deepseek, "sk-x", "deepseek-chat")
	if !plain.DisableThinking {
		t.Error("NewClient: DisableThinking = false for a deepseek base URL, want true")
	}
	if got := plain.DialTimeout(); got != 0 {
		t.Errorf("NewClient: DialTimeout() = %s, want 0 (default transport)", got)
	}

	fast := NewClientWithFastDial(deepseek, "sk-x", "deepseek-chat", 2*time.Second)
	if !fast.DisableThinking {
		t.Error("NewClientWithFastDial: DisableThinking = false for a deepseek base URL, want true")
	}

	// A non-deepseek endpoint keeps thinking enabled in both constructors.
	other := NewClientWithFastDial("https://example.invalid/v1", "sk-x", "m", 2*time.Second)
	if other.DisableThinking {
		t.Error("NewClientWithFastDial: DisableThinking = true for a non-deepseek base URL, want false")
	}
}

// DialTimeout is what makes the fast-fail policy observable: the value stored here
// is the dial timeout that governs how long an unresponsive provider is waited on.
func TestNewClientWithFastDialReportsDialTimeout(t *testing.T) {
	want := 1500 * time.Millisecond
	c := NewClientWithFastDial("https://example.invalid/v1", "sk-x", "m", want)
	if got := c.DialTimeout(); got != want {
		t.Errorf("DialTimeout() = %s, want %s", got, want)
	}
	if c.HTTPClient.Transport == nil {
		t.Error("HTTPClient.Transport = nil, want the custom fast-dial transport")
	}
}
