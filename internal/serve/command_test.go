package serve

import (
	"bytes"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/alecthomas/kong"
	coreserve "github.com/yusiwen/myUtilities/internal/core/serve"
)

func parseServe(t *testing.T, args ...string) (*Options, error) {
	t.Helper()
	root := struct {
		Serve Options `cmd:"" name:"serve"`
	}{}
	parser, err := kong.New(&root)
	if err != nil {
		t.Fatalf("kong.New: %v", err)
	}
	if _, err := parser.Parse(args); err != nil {
		return &root.Serve, err
	}
	return &root.Serve, nil
}

func TestFlagDefaults(t *testing.T) {
	o, err := parseServe(t, "serve")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if o.Path != "." {
		t.Errorf("Path = %q, want .", o.Path)
	}
	if o.Port != 8080 {
		t.Errorf("Port = %d, want 8080", o.Port)
	}
	if !o.QR {
		t.Error("QR should default to true")
	}
	if o.Verbose {
		t.Error("Verbose should default to false")
	}
}

func TestQRCanBeDisabled(t *testing.T) {
	o, err := parseServe(t, "serve", "--no-qr")
	if err != nil {
		t.Fatalf("parse --no-qr: %v", err)
	}
	if o.QR {
		t.Error("--no-qr left QR enabled")
	}

	o, err = parseServe(t, "serve", "--qr=false")
	if err != nil {
		t.Fatalf("parse --qr=false: %v", err)
	}
	if o.QR {
		t.Error("--qr=false left QR enabled")
	}
}

func TestPositionalPathAndShortFlags(t *testing.T) {
	o, err := parseServe(t, "serve", "./dist.zip", "-p", "9999", "--verbose", "--iface", "en0")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if o.Path != "./dist.zip" {
		t.Errorf("Path = %q, want ./dist.zip", o.Path)
	}
	if o.Port != 9999 {
		t.Errorf("Port = %d, want 9999", o.Port)
	}
	if !o.Verbose {
		t.Error("--verbose did not enable Verbose")
	}
	if o.Iface != "en0" {
		t.Errorf("Iface = %q, want en0", o.Iface)
	}
}

func TestResolveTarget(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "data.zip")
	if err := os.WriteFile(file, []byte("payload"), 0o644); err != nil {
		t.Fatal(err)
	}

	t.Run("directory", func(t *testing.T) {
		got, err := resolveTarget(dir)
		if err != nil {
			t.Fatalf("resolveTarget: %v", err)
		}
		if !got.isDir() {
			t.Errorf("kind = %v, want targetDir", got.kind)
		}
		if !filepath.IsAbs(got.path) {
			t.Errorf("path = %q, want an absolute path", got.path)
		}
	})

	t.Run("file", func(t *testing.T) {
		got, err := resolveTarget(file)
		if err != nil {
			t.Fatalf("resolveTarget: %v", err)
		}
		if got.isDir() {
			t.Errorf("kind = %v, want targetFile", got.kind)
		}
		if got.size != int64(len("payload")) {
			t.Errorf("size = %d, want %d", got.size, len("payload"))
		}
	})

	t.Run("missing", func(t *testing.T) {
		if _, err := resolveTarget(filepath.Join(dir, "missing.zip")); err == nil {
			t.Fatal("resolveTarget accepted a missing path")
		}
	})
}

func TestChoosePrimary(t *testing.T) {
	addrs := []coreserve.Addr{
		{IP: net.IPv4(192, 168, 1, 20), Interface: "en0"},
		{IP: net.IPv4(10, 0, 0, 4), Interface: "en1"},
	}

	t.Run("lan", func(t *testing.T) {
		primary, localOnly, rest := choosePrimary("", addrs, 8080)
		if primary != "http://192.168.1.20:8080/" {
			t.Errorf("primary = %q, want the first LAN address", primary)
		}
		if localOnly {
			t.Error("localOnly = true for a LAN address")
		}
		if len(rest) != 1 || rest[0].Interface != "en1" {
			t.Errorf("rest = %v, want the remaining address", rest)
		}
	})

	t.Run("explicit bind address wins", func(t *testing.T) {
		primary, _, rest := choosePrimary("10.1.2.3", addrs, 8080)
		if primary != "http://10.1.2.3:8080/" {
			t.Errorf("primary = %q, want the bind address", primary)
		}
		if len(rest) != 0 {
			t.Errorf("rest = %v, want none: only the bind address is reachable", rest)
		}
	})

	t.Run("wildcard host falls back to LAN", func(t *testing.T) {
		primary, _, _ := choosePrimary("0.0.0.0", addrs, 8080)
		if primary != "http://192.168.1.20:8080/" {
			t.Errorf("primary = %q, want the LAN address", primary)
		}
	})

	t.Run("loopback warns instead of advertising", func(t *testing.T) {
		primary, localOnly, _ := choosePrimary("127.0.0.1", addrs, 8080)
		if primary != "http://127.0.0.1:8080/" {
			t.Errorf("primary = %q, want the loopback URL", primary)
		}
		if !localOnly {
			t.Error("localOnly = false for a loopback bind address")
		}
	})

	t.Run("localhost hostname", func(t *testing.T) {
		primary, localOnly, _ := choosePrimary("localhost", addrs, 8080)
		if primary != "http://localhost:8080/" {
			t.Errorf("primary = %q, want the localhost URL", primary)
		}
		if !localOnly {
			t.Error("localOnly = false for a localhost bind")
		}
	})

	t.Run("no addresses", func(t *testing.T) {
		primary, localOnly, _ := choosePrimary("", nil, 8080)
		if primary != "" || !localOnly {
			t.Errorf("choosePrimary = %q, %v; want no URL and localOnly", primary, localOnly)
		}
	})
}

func TestURLFor(t *testing.T) {
	if got := urlFor("192.168.1.5", 8080); got != "http://192.168.1.5:8080/" {
		t.Errorf("urlFor IPv4 = %q", got)
	}
	if got := urlFor("::1", 8080); got != "http://[::1]:8080/" {
		t.Errorf("urlFor IPv6 = %q, want brackets around the host", got)
	}
}

func TestFormatBytes(t *testing.T) {
	cases := map[int64]string{
		0:       "0 B",
		999:     "999 B",
		1024:    "1.0 KiB",
		1536:    "1.5 KiB",
		5 << 20: "5.0 MiB",
		3 << 30: "3.0 GiB",
	}
	for in, want := range cases {
		if got := formatBytes(in); got != want {
			t.Errorf("formatBytes(%d) = %q, want %q", in, got, want)
		}
	}
}

func TestPrintQRRespectsFlagsAndTerminal(t *testing.T) {
	const url = "http://192.168.1.5:8080/"

	t.Run("terminal with room", func(t *testing.T) {
		var buf bytes.Buffer
		o := &Options{QR: true}
		o.printQRTo(&buf, url, true, 200)
		if !strings.Contains(buf.String(), "Scan with your phone:") {
			t.Error("no prompt printed")
		}
		if !strings.Contains(buf.String(), "\x1b[48;5;") {
			t.Error("no ANSI drawing printed")
		}
	})

	t.Run("piped stdout", func(t *testing.T) {
		var buf bytes.Buffer
		o := &Options{QR: true}
		o.printQRTo(&buf, url, false, 0)
		if buf.Len() != 0 {
			t.Errorf("wrote %d bytes to a non-terminal, want none", buf.Len())
		}
	})

	t.Run("disabled by flag", func(t *testing.T) {
		var buf bytes.Buffer
		o := &Options{QR: false}
		o.printQRTo(&buf, url, true, 200)
		if buf.Len() != 0 {
			t.Errorf("wrote %d bytes with --no-qr, want none", buf.Len())
		}
	})

	t.Run("terminal too narrow", func(t *testing.T) {
		var buf bytes.Buffer
		o := &Options{QR: true}
		o.printQRTo(&buf, url, true, 20)
		if buf.Len() != 0 {
			t.Errorf("wrote a %d-byte drawing that would wrap, want none", buf.Len())
		}
	})

	t.Run("no url", func(t *testing.T) {
		var buf bytes.Buffer
		o := &Options{QR: true}
		o.printQRTo(&buf, "", true, 200)
		if buf.Len() != 0 {
			t.Errorf("wrote %d bytes without a URL, want none", buf.Len())
		}
	})
}

func TestListenReportsActualPort(t *testing.T) {
	o := &Options{Host: "127.0.0.1", Port: 0}
	ln, port, err := o.listen()
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()

	if port == 0 {
		t.Fatal("listen did not report the port actually bound")
	}
	if addr, ok := ln.Addr().(*net.TCPAddr); !ok || addr.Port != port {
		t.Errorf("listener is on %v, reported port %d", ln.Addr(), port)
	}
}

func TestListenFailsBeforeAnnouncingOnBusyPort(t *testing.T) {
	busy, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve a port: %v", err)
	}
	defer busy.Close()
	port := busy.Addr().(*net.TCPAddr).Port

	o := &Options{Host: "127.0.0.1", Port: port}
	if ln, _, err := o.listen(); err == nil {
		ln.Close()
		t.Fatalf("listen on busy port %d succeeded", port)
	} else if !strings.Contains(err.Error(), "listen on") {
		t.Errorf("error = %v, want it to name the address", err)
	}
}

func TestTemporaryShareFlags(t *testing.T) {
	o, err := parseServe(t, "serve", "x.zip", "--token", "abc123", "--once", "--expire", "10m")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if o.Token != "abc123" {
		t.Errorf("Token = %q, want abc123", o.Token)
	}
	if !o.Once {
		t.Error("--once did not enable Once")
	}
	if o.Expire != 10*time.Minute {
		t.Errorf("Expire = %v, want 10m", o.Expire)
	}

	o, err = parseServe(t, "serve", "x.zip", "--random-token")
	if err != nil {
		t.Fatalf("parse --random-token: %v", err)
	}
	if !o.RandomToken {
		t.Error("--random-token did not enable RandomToken")
	}
}

func TestExpandDefaultsToZero(t *testing.T) {
	o, err := parseServe(t, "serve", "x.zip")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if o.Expire != 0 {
		t.Errorf("Expire = %v, want 0 (no expiry)", o.Expire)
	}
	if o.Once || o.RandomToken || o.Token != "" {
		t.Error("temporary-share options should default to off")
	}
}

func TestResolveToken(t *testing.T) {
	t.Run("explicit", func(t *testing.T) {
		o := &Options{Token: "abc123"}
		token, err := o.resolveToken()
		if err != nil {
			t.Fatalf("resolveToken: %v", err)
		}
		if token != "abc123" {
			t.Errorf("token = %q, want abc123", token)
		}
	})

	t.Run("invalid", func(t *testing.T) {
		o := &Options{Token: "with/slash"}
		if _, err := o.resolveToken(); err == nil {
			t.Fatal("resolveToken accepted a token containing a slash")
		}
	})

	t.Run("generated", func(t *testing.T) {
		o := &Options{RandomToken: true}
		token, err := o.resolveToken()
		if err != nil {
			t.Fatalf("resolveToken: %v", err)
		}
		if err := coreserve.ValidateToken(token); err != nil {
			t.Errorf("generated token %q is invalid: %v", token, err)
		}
	})

	t.Run("both flags", func(t *testing.T) {
		o := &Options{Token: "abc", RandomToken: true}
		if _, err := o.resolveToken(); err == nil {
			t.Fatal("resolveToken accepted --token together with --random-token")
		}
	})

	t.Run("neither", func(t *testing.T) {
		o := &Options{}
		token, err := o.resolveToken()
		if err != nil || token != "" {
			t.Errorf("resolveToken = %q, %v; want an empty token and no error", token, err)
		}
	})
}

func TestValidateDirMode(t *testing.T) {
	cases := []struct {
		name    string
		o       Options
		wantErr bool
	}{
		{name: "plain", o: Options{}},
		{name: "token", o: Options{Token: "abc"}, wantErr: true},
		{name: "random token", o: Options{RandomToken: true}, wantErr: true},
		{name: "once", o: Options{Once: true}, wantErr: true},
		{name: "expire", o: Options{Expire: time.Minute}, wantErr: true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := c.o.validateDirMode()
			if (err != nil) != c.wantErr {
				t.Errorf("validateDirMode() = %v, wantErr %v", err, c.wantErr)
			}
		})
	}
}

func TestURLHelpers(t *testing.T) {
	if got := withToken("http://192.168.1.5:8080/", ""); got != "http://192.168.1.5:8080/" {
		t.Errorf("withToken without a token = %q", got)
	}
	if got := withToken("http://192.168.1.5:8080/", "abc"); got != "http://192.168.1.5:8080/abc" {
		t.Errorf("withToken = %q", got)
	}
	if got := joinURL("http://192.168.1.5:8080/abc", "data.zip"); got != "http://192.168.1.5:8080/abc/data.zip" {
		t.Errorf("joinURL = %q", got)
	}
	if got := joinURL("http://192.168.1.5:8080/", "data.zip"); got != "http://192.168.1.5:8080/data.zip" {
		t.Errorf("joinURL with a trailing slash base = %q", got)
	}
}

func TestRedact(t *testing.T) {
	if got := redact("/abc123/data.zip", []string{"abc123"}); got != "/<token>/data.zip" {
		t.Errorf("redact = %q", got)
	}
	if got := redact("/data.zip", nil); got != "/data.zip" {
		t.Errorf("redact without secrets = %q", got)
	}
}

func TestOnCompleteOnlyForSingleUse(t *testing.T) {
	done := make(chan struct{})

	if cb := (&Options{}).onComplete(done); cb != nil {
		t.Fatal("a callback was wired without --once; the server would stop after one download")
	}
	select {
	case <-done:
		t.Fatal("done was closed without --once")
	default:
	}

	o := &Options{Once: true}
	cb := o.onComplete(done)
	if cb == nil {
		t.Fatal("no callback wired for --once")
	}
	// Calling twice must not panic on a double close.
	cb()
	cb()
	select {
	case <-done:
	default:
		t.Fatal("done was not closed after a complete download")
	}
}
