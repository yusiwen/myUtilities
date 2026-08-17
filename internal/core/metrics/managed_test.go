package metrics

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"testing"
	"time"
)

// TestMain lives in tsdb_test.go (it gates the GO_WANT_HELPER re-exec).

// helperServe makes the test binary act as a tiny metrics-shaped server so the
// full spawn lifecycle can be exercised hermetically.
func helperServe() {
	port := "0"
	args := os.Args[1:]
	for i, a := range args {
		if a == "--port" && i+1 < len(args) {
			port = args[i+1]
		}
	}
	ln, err := net.Listen("tcp", "127.0.0.1:"+port)
	if err != nil {
		fmt.Fprintln(os.Stderr, "helper listen:", err)
		os.Exit(1)
	}
	srv := &http.Server{}
	srv.Serve(ln)
}

func findFreePort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	return ln.Addr().(*net.TCPAddr).Port
}

func httptestPort(t *testing.T, srv *httptest.Server) int {
	t.Helper()
	_, portStr, err := net.SplitHostPort(srv.Listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		t.Fatal(err)
	}
	return port
}

func metricsHandler(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/api/metrics" {
		json.NewEncoder(w).Encode([]string{"cpu.used.percent"})
		return
	}
	http.NotFound(w, r)
}

func TestProbeHelpers(t *testing.T) {
	if portListening(findFreePort(t)) {
		t.Fatal("expected free port to not be listening")
	}

	srv := httptest.NewServer(http.HandlerFunc(metricsHandler))
	defer srv.Close()
	port := httptestPort(t, srv)
	if !portListening(port) {
		t.Fatal("expected metrics server port to be listening")
	}
	if !probeMetrics(port) {
		t.Fatal("expected probeMetrics to detect metrics server")
	}

	srv2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "<html>not metrics</html>")
	}))
	defer srv2.Close()
	port2 := httptestPort(t, srv2)
	if probeMetrics(port2) {
		t.Fatal("expected probeMetrics to reject non-metrics server")
	}
}

func TestManagedExternalAndBusyPort(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(metricsHandler))
	defer srv.Close()
	port := httptestPort(t, srv)

	mgr := NewManagedServer(port)
	if err := mgr.Start(); err != nil {
		t.Fatalf("Start against external metrics server: %v", err)
	}
	if st := mgr.Status(); st.State != managedStateExternal {
		t.Fatalf("expected external state, got %s", st.State)
	}
	if err := mgr.Stop(); err != nil {
		t.Fatalf("Stop on external: %v", err)
	}
	if st := mgr.Status(); st.State != managedStateStopped {
		t.Fatalf("expected stopped after Stop, got %s", st.State)
	}

	srv2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "not metrics")
	}))
	defer srv2.Close()
	port2 := httptestPort(t, srv2)

	mgr2 := NewManagedServer(port2)
	if err := mgr2.Start(); err == nil {
		t.Fatal("expected error for busy non-metrics port")
	}
	if st := mgr2.Status(); st.State != managedStateError {
		t.Fatalf("expected error state, got %s", st.State)
	}
}

func TestManagedSpawnLifecycle(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	port := findFreePort(t)

	mgr := NewManagedServer(port)
	mgr.Bin = exe
	mgr.Env = append(os.Environ(), "GO_WANT_HELPER=1")

	if err := mgr.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	waitState(t, mgr, managedStateRunning)
	if st := mgr.Status(); st.Pid == 0 {
		t.Fatalf("expected pid, got %+v", st)
	}
	if !portListening(port) {
		t.Fatal("expected managed server to be listening")
	}

	if err := mgr.Stop(); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if st := mgr.Status(); st.State != managedStateStopped {
		t.Fatalf("expected stopped, got %s", st.State)
	}
	if portListening(port) {
		t.Fatal("expected port to be released after Stop")
	}

	if err := mgr.Restart(); err != nil {
		t.Fatalf("Restart: %v", err)
	}
	waitState(t, mgr, managedStateRunning)
	_ = mgr.Stop()
}

func waitState(t *testing.T, mgr *ManagedServer, want string) {
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for {
		st := mgr.Status()
		if st.State == want {
			return
		}
		if st.State == managedStateError {
			t.Fatalf("manager entered error state: %s (log: %v)", st.Error, st.Log)
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s, state=%s (log: %v)", want, st.State, st.Log)
		}
		time.Sleep(100 * time.Millisecond)
	}
}
