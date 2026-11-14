package browser

import (
	"errors"
	"testing"
)

// fakeLookPath returns a resolver that reports only the named binaries.
func fakeLookPath(available ...string) func(string) (string, error) {
	paths := make(map[string]string, len(available))
	for _, name := range available {
		paths[name] = "/mock/" + name
	}
	return func(name string) (string, error) {
		if p, ok := paths[name]; ok {
			return p, nil
		}
		return "", errors.New("not found")
	}
}

func TestCommandDarwin(t *testing.T) {
	argv, err := command("darwin", fakeLookPath("open"), "http://127.0.0.1:8088/")
	if err != nil {
		t.Fatalf("command: %v", err)
	}
	want := []string{"/mock/open", "http://127.0.0.1:8088/"}
	assertArgv(t, argv, want)
}

func TestCommandDarwinWithoutOpen(t *testing.T) {
	if _, err := command("darwin", fakeLookPath(), "http://x/"); err == nil {
		t.Fatal("expected an error when open is missing")
	}
}

func TestCommandWindows(t *testing.T) {
	url := "http://127.0.0.1:8088/?a=1&b=2"
	argv, err := command("windows", fakeLookPath("rundll32"), url)
	if err != nil {
		t.Fatalf("command: %v", err)
	}
	// The URL must stay a single argument so & and ^ reach the browser intact.
	want := []string{"/mock/rundll32", "url.dll,FileProtocolHandler", url}
	assertArgv(t, argv, want)
}

func TestCommandLinuxPrefersXdgOpen(t *testing.T) {
	argv, err := command("linux", fakeLookPath("gio", "xdg-open"), "http://x/")
	if err != nil {
		t.Fatalf("command: %v", err)
	}
	assertArgv(t, argv, []string{"/mock/xdg-open", "http://x/"})
}

func TestCommandLinuxFallsBackToGio(t *testing.T) {
	argv, err := command("linux", fakeLookPath("gio"), "http://x/")
	if err != nil {
		t.Fatalf("command: %v", err)
	}
	assertArgv(t, argv, []string{"/mock/gio", "open", "http://x/"})
}

func TestCommandLinuxNoLauncher(t *testing.T) {
	if _, err := command("linux", fakeLookPath(), "http://x/"); err == nil {
		t.Fatal("expected an error when no launcher exists")
	}
}

func assertArgv(t *testing.T, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("argv = %q, want %q", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("argv[%d] = %q, want %q (argv = %q)", i, got[i], want[i], got)
		}
	}
}
