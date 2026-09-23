package fleet

import "testing"

// TestValidateServeAuth pins the dispatcher start-up rules: a token is required
// unless anonymous mode was requested explicitly, and the two cannot be
// combined (so a configured token is never silently ignored).
func TestValidateServeAuth(t *testing.T) {
	tests := []struct {
		name           string
		token          string
		allowAnonymous bool
		wantErr        bool
	}{
		{name: "token configured", token: "s3cret"},
		{name: "anonymous opt-in", allowAnonymous: true},
		{name: "no token and no opt-in", wantErr: true},
		{name: "token plus anonymous opt-in", token: "s3cret", allowAnonymous: true, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateServeAuth(tt.token, tt.allowAnonymous)
			if tt.wantErr && err == nil {
				t.Fatal("expected an error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

// TestResolveToken covers the --token overrides config precedence used by the
// dispatcher, the agent and every client command.
func TestResolveToken(t *testing.T) {
	tests := []struct {
		name string
		opts CommonOptions
		cfg  *Config
		want string
	}{
		{name: "flag wins", opts: CommonOptions{Token: "from-flag"}, cfg: &Config{Token: "from-config"}, want: "from-flag"},
		{name: "config fallback", cfg: &Config{Token: "from-config"}, want: "from-config"},
		{name: "neither set", cfg: &Config{}, want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.opts.resolveToken(tt.cfg); got != tt.want {
				t.Fatalf("resolveToken() = %q, want %q", got, tt.want)
			}
		})
	}
}
