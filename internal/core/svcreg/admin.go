package svcreg

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
)

func expandTilde(p string) string {
	if strings.HasPrefix(p, "~/") {
		home, err := os.UserHomeDir()
		if err == nil {
			return filepath.Join(home, p[2:])
		}
	}
	return p
}

func processExists(pid int) bool {
	if pid <= 0 {
		return false
	}
	if _, err := os.Stat(fmt.Sprintf("/proc/%d", pid)); err == nil {
		return true
	}
	if p, err := os.FindProcess(pid); err == nil {
		return p.Signal(syscall.Signal(0)) == nil
	}
	return false
}

type adminConfig struct {
	Port        int    `json:"port"`
	Host        string `json:"host"`
	DBPath      string `json:"dbPath"`
	Independent bool   `json:"independent"`
}

type adminState struct {
	PID    int         `json:"pid"`
	Config adminConfig `json:"config"`
}

// ServerManager manages the lifecycle of an svcreg serve subprocess.
type ServerManager struct {
	mu      sync.Mutex
	cmd     *exec.Cmd
	pidFile string
	running bool
	config  adminConfig
	pid     int
}

var mgr = NewServerManager()

var stateDir string

// dataRoot is the only directory the admin API may write to: the configured
// config dir when one was given, otherwise ~/.config/mu. It anchors the state
// file, the serve log and every client-supplied db path.
func dataRoot() string {
	if stateDir != "" {
		return stateDir
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "/tmp"
	}
	return filepath.Join(home, ".config", "mu")
}

func adminStatePath() string {
	return filepath.Join(dataRoot(), "svcreg-admin.json")
}

func NewServerManager() *ServerManager {
	return &ServerManager{pidFile: adminStatePath()}
}

// RestoreState restores a previously running server from its state file.
func RestoreState(configDir string) {
	if configDir != "" {
		stateDir = configDir
	}
	os.MkdirAll(stateDir, 0700)
	mgr.pidFile = adminStatePath()
	mgr.restoreState()
}

func (m *ServerManager) restoreState() {
	data, err := os.ReadFile(m.pidFile)
	if err != nil {
		log.Printf("svcreg: restoreState read %s: %v", m.pidFile, err)
		return
	}
	var state adminState
	if err := json.Unmarshal(data, &state); err != nil {
		log.Printf("svcreg: restoreState unmarshal: %v", err)
		return
	}
	if state.PID <= 0 {
		return
	}
	if !processExists(state.PID) {
		log.Printf("svcreg: restoreState PID %d not alive, clearing state", state.PID)
		m.saveClearedState()
		return
	}
	addr := fmt.Sprintf("127.0.0.1:%d", state.Config.Port)
	conn, err := net.DialTimeout("tcp", addr, 2*time.Second)
	if err != nil {
		log.Printf("svcreg: restoreState TCP check %s failed: %v, clearing state", addr, err)
		m.saveClearedState()
		return
	}
	conn.Close()
	m.running = true
	m.pid = state.PID
	m.config = state.Config
	log.Printf("svcreg: restoreState SUCCESS (PID: %d, port: %d)", state.PID, state.Config.Port)
}

func (m *ServerManager) saveState() {
	state := adminState{PID: m.pid, Config: m.config}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		log.Printf("svcreg: saveState marshal: %v", err)
		return
	}
	dir := filepath.Dir(m.pidFile)
	if err := os.MkdirAll(dir, 0700); err != nil {
		log.Printf("svcreg: saveState mkdir: %v", err)
		return
	}
	if err := os.WriteFile(m.pidFile, data, 0600); err != nil {
		log.Printf("svcreg: saveState write: %v", err)
	}
}

func (m *ServerManager) saveClearedState() {
	state := adminState{PID: -1, Config: adminConfig{}}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return
	}
	dir := filepath.Dir(m.pidFile)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return
	}
	os.WriteFile(m.pidFile, data, 0600)
}

// logPath is a fixed location under the data root. It is deliberately not
// derived from request data, so a caller cannot choose where the server writes.
func (m *ServerManager) logPath() string {
	return filepath.Join(dataRoot(), "svcreg-serve.log")
}

// resolveDBPath validates a client-supplied database path: a relative path is
// taken as relative to the data root, and the result must stay inside the data
// root, so the admin API cannot create or open a database anywhere on the
// filesystem.
func resolveDBPath(raw string) (string, error) {
	root, err := filepath.Abs(dataRoot())
	if err != nil {
		return "", fmt.Errorf("invalid data dir: %w", err)
	}
	p := expandTilde(raw)
	if !filepath.IsAbs(p) {
		p = filepath.Join(root, p)
	}
	abs, err := filepath.Abs(p)
	if err != nil {
		return "", fmt.Errorf("invalid dbPath %q: %w", raw, err)
	}
	rel, err := filepath.Rel(root, abs)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("dbPath %q must be inside %s", raw, root)
	}
	return abs, nil
}

// validateAdminConfig rejects request values that are unsafe to pass to the
// child process.
func validateAdminConfig(cfg *adminConfig) error {
	if cfg.Port < 1 || cfg.Port > 65535 {
		return fmt.Errorf("port %d out of range", cfg.Port)
	}
	if cfg.Host == "" || strings.HasPrefix(cfg.Host, "-") {
		return fmt.Errorf("invalid host %q", cfg.Host)
	}
	// Only literal addresses and plain hostnames: the value is passed to the
	// child as an argv element, so anything exotic is unnecessary.
	for _, r := range cfg.Host {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		case r == '.', r == ':', r == '-', r == '_':
		default:
			return fmt.Errorf("invalid host %q", cfg.Host)
		}
	}
	dbPath, err := resolveDBPath(cfg.DBPath)
	if err != nil {
		return err
	}
	cfg.DBPath = dbPath
	return nil
}

func (m *ServerManager) readLogs() []string {
	path := m.logPath()
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	var lines []string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	if len(lines) > 200 {
		lines = lines[len(lines)-200:]
	}
	return lines
}

// Status returns the current server manager state.
func (m *ServerManager) Status() map[string]interface{} {
	m.mu.Lock()
	defer m.mu.Unlock()
	return map[string]interface{}{
		"running": m.running,
		"pid":     m.pid,
		"config":  m.config,
		"logs":    m.readLogs(),
	}
}

// Start launches a new svcreg serve subprocess.
func (m *ServerManager) Start(cfg adminConfig) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.running {
		return fmt.Errorf("server is already running (PID: %d)", m.pid)
	}

	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("cannot find executable: %w", err)
	}

	args := []string{"svcreg", "serve",
		"--port", fmt.Sprintf("%d", cfg.Port),
		"--host", cfg.Host,
		"--db-path", cfg.DBPath,
	}

	cmd := exec.Command(exe, args...)

	logPath := m.logPath()
	lf, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		return fmt.Errorf("cannot open log file %s: %w", logPath, err)
	}
	cmd.Stdout = lf
	cmd.Stderr = lf

	if cfg.Independent {
		setIndependent(cmd)
	}

	if err := cmd.Start(); err != nil {
		lf.Close()
		return fmt.Errorf("start: %w", err)
	}

	m.cmd = cmd
	m.running = true
	m.config = cfg
	m.pid = cmd.Process.Pid
	m.saveState()

	go func() {
		cmd.Wait()
		lf.Close()
		m.mu.Lock()
		m.running = false
		m.pid = 0
		m.cmd = nil
		m.saveClearedState()
		m.mu.Unlock()
	}()

	return nil
}

// Stop terminates the running server subprocess.
func (m *ServerManager) Stop() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.running {
		return fmt.Errorf("server is not running")
	}

	log.Printf("Stopping server (PID: %d)...", m.pid)
	p, err := os.FindProcess(m.pid)
	if err == nil {
		p.Signal(os.Interrupt)
		time.Sleep(2 * time.Second)
		p.Kill()
	}
	if m.cmd != nil {
		m.cmd.Wait()
	}
	m.running = false
	m.pid = 0
	m.cmd = nil
	m.saveClearedState()
	return nil
}

// adminOnly restricts a handler to requests that arrive over the loopback
// interface.
//
// The admin API controls the local svcreg subprocess (start/stop, db path, port,
// log file) and is also proxied by the gateway, so a remote caller must never be
// able to drive it. svcreg has no token concept, and the dashboard is used from
// the same host as the server, so loopback is the boundary.
func adminOnly(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !isLoopbackRequest(r) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusForbidden)
			json.NewEncoder(w).Encode(map[string]string{
				"error": "the svcreg admin API is only available from the local host",
			})
			return
		}
		next(w, r)
	}
}

// isLoopbackRequest reports whether the request came from the local host.
func isLoopbackRequest(r *http.Request) bool {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return false
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// RegisterAdminAPI registers the admin lifecycle endpoints. Every route is
// loopback-only (see adminOnly).
func RegisterAdminAPI(mux *http.ServeMux, client *Client) {
	if mgr.running && mgr.config.Port > 0 {
		client.SetServer(fmt.Sprintf("http://127.0.0.1:%d", mgr.config.Port))
	}
	mux.HandleFunc("/api/svcreg/admin/config", adminOnly(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"defaultPort":   30100,
			"defaultHost":   "0.0.0.0",
			"defaultDBPath": "~/.config/mu/svcreg.db",
		})
	}))
	mux.HandleFunc("/api/svcreg/admin/status", adminOnly(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if mgr.running {
			client.SetServer(fmt.Sprintf("http://127.0.0.1:%d", mgr.config.Port))
		}
		json.NewEncoder(w).Encode(mgr.Status())
	}))
	mux.HandleFunc("/api/svcreg/admin/start", adminOnly(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			WriteProxyError(w, fmt.Errorf("POST required"))
			return
		}
		var cfg adminConfig
		if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
			WriteProxyError(w, fmt.Errorf("invalid config: %w", err))
			return
		}
		if cfg.Port == 0 {
			cfg.Port = 30100
		}
		if cfg.Host == "" {
			cfg.Host = "0.0.0.0"
		}
		if cfg.DBPath == "" {
			cfg.DBPath = "~/.config/mu/svcreg.db"
		}
		if err := validateAdminConfig(&cfg); err != nil {
			WriteProxyError(w, err)
			return
		}
		if err := mgr.Start(cfg); err != nil {
			WriteProxyError(w, err)
			return
		}
		client.SetServer(fmt.Sprintf("http://127.0.0.1:%d", cfg.Port))
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "started"})
	}))
	mux.HandleFunc("/api/svcreg/admin/stop", adminOnly(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			WriteProxyError(w, fmt.Errorf("POST required"))
			return
		}
		if err := mgr.Stop(); err != nil {
			WriteProxyError(w, err)
			return
		}
		client.SetServer("http://127.0.0.1:30100")
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "stopped"})
	}))
}
