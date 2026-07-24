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

type serverManager struct {
	mu      sync.Mutex
	cmd     *exec.Cmd
	pidFile string
	running bool
	config  adminConfig
	pid     int
}

var mgr = newServerManager()

func adminStatePath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "/tmp/svcreg-admin.json"
	}
	return filepath.Join(home, ".config", "mu", "svcreg-admin.json")
}

func newServerManager() *serverManager {
	return &serverManager{pidFile: adminStatePath()}
}

func RestoreState() {
	mgr.restoreState()
}

func (m *serverManager) restoreState() {
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

func (m *serverManager) saveState() {
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

func (m *serverManager) saveClearedState() {
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

func (m *serverManager) logPath() string {
	return expandTilde(m.config.DBPath) + ".log"
}

func (m *serverManager) readLogs() []string {
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

func (m *serverManager) status() map[string]interface{} {
	m.mu.Lock()
	defer m.mu.Unlock()
	return map[string]interface{}{
		"running": m.running,
		"pid":     m.pid,
		"config":  m.config,
		"logs":    m.readLogs(),
	}
}

func (m *serverManager) start(cfg adminConfig) error {
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

	logPath := expandTilde(cfg.DBPath) + ".log"
	lf, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
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

func (m *serverManager) stop() error {
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

func registerAdminAPI(mux *http.ServeMux, client *Client) {
	if mgr.running && mgr.config.Port > 0 {
		client.Server = fmt.Sprintf("http://127.0.0.1:%d", mgr.config.Port)
	}
	mux.HandleFunc("/api/svcreg/admin/config", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"defaultPort":   30100,
			"defaultHost":   "0.0.0.0",
			"defaultDBPath": "~/.config/mu/svcreg.db",
		})
	})
	mux.HandleFunc("/api/svcreg/admin/status", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if mgr.running {
			client.Server = fmt.Sprintf("http://127.0.0.1:%d", mgr.config.Port)
		}
		json.NewEncoder(w).Encode(mgr.status())
	})
	mux.HandleFunc("/api/svcreg/admin/start", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeProxyError(w, fmt.Errorf("POST required"))
			return
		}
		var cfg adminConfig
		if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
			writeProxyError(w, fmt.Errorf("invalid config: %w", err))
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
		if err := mgr.start(cfg); err != nil {
			writeProxyError(w, err)
			return
		}
		client.Server = fmt.Sprintf("http://127.0.0.1:%d", cfg.Port)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "started"})
	})
	mux.HandleFunc("/api/svcreg/admin/stop", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeProxyError(w, fmt.Errorf("POST required"))
			return
		}
		if err := mgr.stop(); err != nil {
			writeProxyError(w, err)
			return
		}
		client.Server = "http://127.0.0.1:30100"
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "stopped"})
	})
}
