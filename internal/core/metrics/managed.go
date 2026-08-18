package metrics

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

const (
	managedStateStopped  = "stopped"
	managedStateStarting = "starting"
	managedStateRunning  = "running"
	managedStateExternal = "external"
	managedStateError    = "error"
)

// ManagedStatus describes the state of a gateway-managed metrics server.
type ManagedStatus struct {
	State string   `json:"state"`
	Pid   int      `json:"pid,omitempty"`
	Since string   `json:"since,omitempty"`
	Port  int      `json:"port"`
	Error string   `json:"error,omitempty"`
	Log   []string `json:"log,omitempty"`
}

// ManagedServer supervises a `mu metrics serve` subprocess. It starts the
// current executable (the `mu` binary) with `metrics serve --port N --agent`,
// captures its output, tracks crashes, and can start/stop/restart it.
type ManagedServer struct {
	// Bin is the path to the mu binary; defaults to os.Executable().
	Bin string
	// Port the managed server should listen on.
	Port int
	// ConfigDir is passed through as --config-dir to the subprocess; it controls
	// both the metrics config file location and the default DB directory.
	ConfigDir string
	// Args are extra arguments appended after the default serve args.
	Args []string
	// Env overrides the child environment (defaults to os.Environ()).
	Env []string

	mu       sync.Mutex
	cmd      *exec.Cmd
	state    string
	pid      int
	since    time.Time
	errMsg   string
	logBuf   []string
	logMax   int
	stopping bool
	exitCh   chan struct{}
}

func NewManagedServer(port int) *ManagedServer {
	return &ManagedServer{
		Port:   port,
		state:  managedStateStopped,
		logMax: 200,
	}
}

func (m *ManagedServer) BinPath() string {
	if m.Bin != "" {
		return m.Bin
	}
	if exe, err := os.Executable(); err == nil {
		return exe
	}
	return "mu"
}

// Start probes the port and, if free, spawns the managed subprocess. If the
// port is already served by a metrics server it enters the "external" state;
// if it is occupied by a non-metrics service it returns an error.
func (m *ManagedServer) Start() error {
	m.mu.Lock()
	if m.cmd != nil && m.cmd.Process != nil {
		m.mu.Unlock()
		return nil
	}

	port := m.Port
	if portListening(port) {
		if probeMetrics(port) {
			m.state = managedStateExternal
			m.errMsg = ""
			m.mu.Unlock()
			return nil
		}
		m.state = managedStateError
		m.errMsg = fmt.Sprintf("port %d is occupied by a non-metrics service", port)
		m.mu.Unlock()
		return fmt.Errorf("%s", m.errMsg)
	}

	m.state = managedStateStarting
	m.errMsg = ""
	m.pid = 0
	m.stopping = false
	cfgArg := ""
	if m.ConfigDir != "" {
		cfgArg = " --config-dir " + m.ConfigDir
	}
	m.logLocked("starting %s metrics serve --port %d --agent%s", m.BinPath(), port, cfgArg)

	args := []string{"metrics", "serve", "--port", strconv.Itoa(port), "--agent"}
	if m.ConfigDir != "" {
		args = append(args, "--config-dir", m.ConfigDir)
	}
	args = append(args, m.Args...)
	cmd := exec.Command(m.BinPath(), args...)
	if len(m.Env) > 0 {
		cmd.Env = m.Env
	} else {
		cmd.Env = os.Environ()
	}
	cmd.SysProcAttr = managedSysProcAttr()

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		m.state = managedStateError
		m.errMsg = err.Error()
		m.mu.Unlock()
		return err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		m.state = managedStateError
		m.errMsg = err.Error()
		m.mu.Unlock()
		return err
	}

	if err := cmd.Start(); err != nil {
		m.state = managedStateError
		m.errMsg = err.Error()
		m.mu.Unlock()
		return err
	}

	m.cmd = cmd
	m.pid = cmd.Process.Pid
	m.since = time.Now()
	m.exitCh = make(chan struct{})
	cmdRef := cmd
	exitCh := m.exitCh
	m.logLocked("started pid %d", m.pid)
	m.mu.Unlock()

	go m.scanOutput(stdout)
	go m.scanOutput(stderr)
	go m.watch(cmdRef, exitCh)
	go m.waitReady(cmdRef, exitCh, port)
	return nil
}

// Stop gracefully terminates the managed subprocess (SIGTERM, then SIGKILL
// after a timeout).
func (m *ManagedServer) Stop() error {
	m.mu.Lock()
	m.stopping = true
	cmd := m.cmd
	if cmd == nil {
		m.state = managedStateStopped
		m.errMsg = ""
		m.mu.Unlock()
		return nil
	}
	proc := cmd.Process
	exitCh := m.exitCh
	m.mu.Unlock()

	if proc != nil {
		m.logf("stopping pid %d", proc.Pid)
		proc.Signal(syscall.SIGTERM)
		select {
		case <-exitCh:
		case <-time.After(5 * time.Second):
			m.logf("no exit after 5s, killing pid %d", proc.Pid)
			proc.Kill()
			<-exitCh
		}
	}

	m.mu.Lock()
	m.cmd = nil
	m.pid = 0
	m.since = time.Time{}
	m.stopping = false
	m.state = managedStateStopped
	m.errMsg = ""
	m.mu.Unlock()
	return nil
}

func (m *ManagedServer) Restart() error {
	if err := m.Stop(); err != nil {
		return err
	}
	return m.Start()
}

func (m *ManagedServer) Status() ManagedStatus {
	m.mu.Lock()
	defer m.mu.Unlock()
	st := ManagedStatus{
		State: m.state,
		Pid:   m.pid,
		Port:  m.Port,
		Error: m.errMsg,
		Log:   append([]string(nil), m.logBuf...),
	}
	if !m.since.IsZero() {
		st.Since = m.since.Format(time.RFC3339)
	}
	return st
}

// watch is the single owner of cmd.Wait(). It closes exitCh and updates state
// when the process exits.
func (m *ManagedServer) watch(cmd *exec.Cmd, exitCh chan struct{}) {
	err := cmd.Wait()

	m.mu.Lock()
	defer m.mu.Unlock()
	close(exitCh)
	if m.cmd != cmd {
		return
	}
	m.cmd = nil
	m.pid = 0
	if m.stopping {
		return
	}
	if err != nil {
		m.state = managedStateError
		m.errMsg = fmt.Sprintf("managed metrics server exited: %v", err)
		m.logLocked("process exited: %v", err)
	} else {
		m.state = managedStateStopped
		m.logLocked("process exited cleanly")
	}
}

// waitReady flips "starting" to "running" once the port accepts connections.
func (m *ManagedServer) waitReady(cmd *exec.Cmd, exitCh chan struct{}, port int) {
	tick := time.NewTicker(300 * time.Millisecond)
	defer tick.Stop()
	deadline := time.After(10 * time.Second)
	for {
		select {
		case <-exitCh:
			return
		case <-deadline:
			m.markRunning(cmd, port)
			return
		case <-tick.C:
			if portListening(port) {
				m.markRunning(cmd, port)
				return
			}
		}
	}
}

func (m *ManagedServer) markRunning(cmd *exec.Cmd, port int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.cmd == cmd && m.state == managedStateStarting {
		m.state = managedStateRunning
		m.logLocked("listening on port %d", port)
	}
}

func (m *ManagedServer) scanOutput(rd io.Reader) {
	sc := bufio.NewScanner(rd)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	for sc.Scan() {
		m.logf("%s", sc.Text())
	}
}

func (m *ManagedServer) logLocked(format string, args ...interface{}) {
	m.logBuf = append(m.logBuf, fmt.Sprintf(format, args...))
	if len(m.logBuf) > m.logMax {
		m.logBuf = append([]string(nil), m.logBuf[len(m.logBuf)-m.logMax:]...)
	}
}

func (m *ManagedServer) logf(format string, args ...interface{}) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.logLocked(format, args...)
}

// portListening reports whether something is accepting TCP connections on the
// given localhost port.
func portListening(port int) bool {
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), 400*time.Millisecond)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

// PortListening reports whether something is accepting TCP connections on the
// given localhost port. Exported for the `mu metrics status` command.
func PortListening(port int) bool {
	return portListening(port)
}

// probeMetrics reports whether the service on the given port looks like a mu
// metrics server (GET /api/metrics returns a JSON string array).
func probeMetrics(port int) bool {
	client := &http.Client{Timeout: 1 * time.Second}
	resp, err := client.Get(fmt.Sprintf("http://127.0.0.1:%d/api/metrics", port))
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return false
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return false
	}
	var names []string
	if err := json.Unmarshal(body, &names); err != nil {
		return false
	}
	return true
}

// ProbeMetrics reports whether the service on the given port looks like a mu
// metrics server. Exported for the `mu metrics status` command.
func ProbeMetrics(port int) bool {
	return probeMetrics(port)
}

// FetchMetricNames returns the metric name list from a running metrics server.
func FetchMetricNames(baseURL string) ([]string, error) {
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get(strings.TrimRight(baseURL, "/") + "/api/metrics")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("metrics server returned %d", resp.StatusCode)
	}
	var names []string
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&names); err != nil {
		return nil, err
	}
	return names, nil
}

// FetchHosts returns the host list from a running metrics server.
func FetchHosts(baseURL string) ([]string, error) {
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get(strings.TrimRight(baseURL, "/") + "/api/metrics/hosts")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("metrics server returned %d", resp.StatusCode)
	}
	var hosts []string
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&hosts); err != nil {
		return nil, err
	}
	return hosts, nil
}
