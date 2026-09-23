package httpserver

import (
	"bufio"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// TestNewSetsTimeouts pins the defaults: every server built here must bound the
// header, body, response and idle phases.
func TestNewSetsTimeouts(t *testing.T) {
	srv := New("127.0.0.1:0", http.NewServeMux())
	defer srv.Close()

	if srv.ReadHeaderTimeout != DefaultReadHeaderTimeout {
		t.Errorf("ReadHeaderTimeout = %v, want %v", srv.ReadHeaderTimeout, DefaultReadHeaderTimeout)
	}
	if srv.ReadTimeout != DefaultReadTimeout {
		t.Errorf("ReadTimeout = %v, want %v", srv.ReadTimeout, DefaultReadTimeout)
	}
	if srv.WriteTimeout != DefaultWriteTimeout {
		t.Errorf("WriteTimeout = %v, want %v", srv.WriteTimeout, DefaultWriteTimeout)
	}
	if srv.IdleTimeout != DefaultIdleTimeout {
		t.Errorf("IdleTimeout = %v, want %v", srv.IdleTimeout, DefaultIdleTimeout)
	}
	if srv.ReadHeaderTimeout <= 0 || srv.ReadTimeout <= 0 || srv.WriteTimeout <= 0 || srv.IdleTimeout <= 0 {
		t.Fatal("no timeout may be left at zero: that is what makes slowloris possible")
	}
}

// TestNewWithHonoursOverrides checks the escape hatch used by callers that need
// a different budget.
func TestNewWithHonoursOverrides(t *testing.T) {
	srv := NewWith("127.0.0.1:0", http.NewServeMux(), Options{
		ReadHeaderTimeout: time.Second,
		IdleTimeout:       3 * time.Second,
	})
	defer srv.Close()

	if srv.ReadHeaderTimeout != time.Second {
		t.Errorf("ReadHeaderTimeout = %v, want 1s", srv.ReadHeaderTimeout)
	}
	if srv.IdleTimeout != 3*time.Second {
		t.Errorf("IdleTimeout = %v, want 3s", srv.IdleTimeout)
	}
	if srv.ReadTimeout != 0 || srv.WriteTimeout != 0 {
		t.Errorf("unset budgets must stay disabled, got read=%v write=%v", srv.ReadTimeout, srv.WriteTimeout)
	}
}

// TestListenAndServeClosesSlowHeaders is the behavioural check: a client that
// opens a connection and never finishes its request headers must be dropped
// after ReadHeaderTimeout instead of holding the server.
func TestListenAndServeClosesSlowHeaders(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	srv := NewWith(ln.Addr().String(), http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "ok")
	}), Options{ReadHeaderTimeout: 300 * time.Millisecond})
	defer srv.Close()
	go srv.Serve(ln)

	conn, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	// Send an incomplete request line + header and then stall.
	if _, err := conn.Write([]byte("GET / HTTP/1.1\r\nHost: x\r\n")); err != nil {
		t.Fatal(err)
	}

	deadline := time.Now().Add(5 * time.Second)
	if err := conn.SetReadDeadline(deadline); err != nil {
		t.Fatal(err)
	}
	reader := bufio.NewReader(conn)
	if _, err := reader.ReadString('\n'); err == nil {
		t.Fatal("expected the server to close the stalled connection, got a response")
	}

	// A well-behaved request still works.
	req, _ := http.NewRequest(http.MethodGet, "http://"+ln.Addr().String()+"/", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("normal request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("normal request: got %d, want 200", resp.StatusCode)
	}
}

// TestListenAndServeIsUsable keeps the helper honest for a plain httptest
// server too.
func TestListenAndServeIsUsable(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "hello")
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("got %d, want 200", resp.StatusCode)
	}
}
