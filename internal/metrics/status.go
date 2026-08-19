package metrics

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	coremetrics "github.com/yusiwen/myUtilities/internal/core/metrics"
)

// runStatusError is returned when no metrics process is found (exit 1).
type runStatusError struct{ msg string }

func (e *runStatusError) Error() string { return e.msg }

func (o *StatusOptions) Run() error {
	if o.Server != "" {
		return o.reportRemote()
	}

	// Unix sockets are authoritative for local process state.
	serverInfo, agentInfo := readLocalSockets()

	// HTTP fallback covers older binaries without a socket.
	if serverInfo == nil {
		base := fmt.Sprintf("http://localhost:%d", o.Port)
		serverInfo = fetchServerInfo(base)
	}

	switch {
	case serverInfo != nil:
		printServer(fmt.Sprintf("http://localhost:%d", o.Port), serverInfo, agentInfo)
		return nil
	case agentInfo != nil:
		printAgent(agentInfo)
		return nil
	default:
		return &runStatusError{fmt.Sprintf("no running metrics server found on http://localhost:%d", o.Port)}
	}
}

func (o *StatusOptions) reportRemote() error {
	base := strings.TrimRight(o.Server, "/")
	info := fetchServerInfo(base)
	if info == nil {
		return &runStatusError{fmt.Sprintf("no running metrics server found on %s", base)}
	}
	printServer(base, info, nil)
	return nil
}

// readLocalSockets dials the per-process Unix sockets under udsDir and decodes
// the payloads.
func readLocalSockets() (*coremetrics.ServerInfo, *coremetrics.ServerInfo) {
	var serverInfo, agentInfo *coremetrics.ServerInfo
	for _, entry := range []struct {
		name string
		out  **coremetrics.ServerInfo
	}{
		{"metrics.sock", &serverInfo},
		{"agent.sock", &agentInfo},
	} {
		data, err := dialUDS(filepath.Join(udsDir, entry.name))
		if err != nil {
			continue
		}
		var info coremetrics.ServerInfo
		if json.Unmarshal(data, &info) != nil {
			continue
		}
		*entry.out = &info
	}
	return serverInfo, agentInfo
}

func dialUDS(path string) ([]byte, error) {
	conn, err := net.DialTimeout("unix", path, 500*time.Millisecond)
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(3 * time.Second))
	buf := make([]byte, 0, 4096)
	tmp := make([]byte, 4096)
	for {
		n, err := conn.Read(tmp)
		buf = append(buf, tmp[:n]...)
		if n == 0 {
			break
		}
		if err != nil {
			break
		}
		if len(buf) > 1<<20 {
			break
		}
	}
	return buf, nil
}

// fetchServerInfo queries the info endpoint; on older servers that lack it,
// it falls back to the names endpoint and returns a minimal info.
func fetchServerInfo(base string) *coremetrics.ServerInfo {
	if resp, err := httpGet(base + "/api/metrics/info"); err == nil {
		var info coremetrics.ServerInfo
		if json.Unmarshal(resp, &info) == nil {
			return &info
		}
	}
	if _, err := coremetrics.FetchMetricNames(base); err == nil {
		return &coremetrics.ServerInfo{Mode: "older-server"}
	}
	return nil
}

func printServer(base string, info *coremetrics.ServerInfo, agent *coremetrics.ServerInfo) {
	fmt.Println("Config:")
	fmt.Printf("  mode             %s\n", info.Mode)
	if info.Pid > 0 {
		fmt.Printf("  pid              %d\n", info.Pid)
	}
	if info.StartedAt != "" {
		fmt.Printf("  started_at       %s\n", info.StartedAt)
	}
	if info.Version != "" {
		fmt.Printf("  version          %s\n", info.Version)
	}
	fmt.Printf("  config-dir       %s\n", displayStr(info.ConfigDir, "(default ~/.config/mu)"))
	fmt.Printf("  config file      %s (%s)\n", displayStr(info.ConfigFile, "-"), existsMark(info.ConfigFile))
	fmt.Printf("  retention        %s\n", displayRetention(info.Retention))
	fmt.Printf("  compact_interval %s\n", displayStr(info.CompactInterval, "1d"))
	fmt.Printf("  collect_interval %s\n", displayStr(info.CollectInterval, "30s"))
	fmt.Printf("  hostname         %s\n", displayStr(info.Hostname, "-"))
	fmt.Printf("  db-path          %s\n", displayStr(info.DBPath, "-"))
	fmt.Printf("  debug            %v\n", info.Debug)
	if agent != nil {
		fmt.Printf("  agent            running (pid %d, %s)\n", agent.Pid, agent.Mode)
	}
	fmt.Println()

	fmt.Println("Running:")
	fmt.Printf("  server           %s\n", base)
	names, namesErr := coremetrics.FetchMetricNames(base)
	if namesErr != nil {
		fmt.Printf("  state            running (info only; HTTP query unavailable)\n")
	} else {
		fmt.Printf("  state            running (%d metrics)\n", len(names))
	}
	fmt.Println()

	fmt.Println("DB:")
	if info.DBPath != "" {
		if st, statErr := os.Stat(info.DBPath); statErr == nil {
			fmt.Printf("  file             %s\n", info.DBPath)
			fmt.Printf("  size             %s\n", humanSize(st.Size()))
			fmt.Printf("  modified         %s\n", st.ModTime().Format("2006-01-02 15:04:05 MST"))
		} else {
			fmt.Printf("  file             %s (not found)\n", info.DBPath)
		}
	}
	if info.Series > 0 || info.Points > 0 {
		fmt.Printf("  series           %d\n", info.Series)
		fmt.Printf("  points           %d\n", info.Points)
	}
	if namesErr == nil {
		hosts, _ := coremetrics.FetchHosts(base)
		fmt.Printf("  hosts            %s\n", listPreview(hosts))
		fmt.Printf("  metrics          %s (%d total)\n", listPreview(names), len(names))
	}
}

func printAgent(info *coremetrics.ServerInfo) {
	fmt.Println("Agent:")
	fmt.Printf("  mode             %s\n", info.Mode)
	fmt.Printf("  pid              %d\n", info.Pid)
	if info.StartedAt != "" {
		fmt.Printf("  started_at       %s\n", info.StartedAt)
	}
	fmt.Printf("  version          %s\n", info.Version)
	fmt.Printf("  config-dir       %s\n", displayStr(info.ConfigDir, "(default ~/.config/mu)"))
	if info.Server != "" {
		fmt.Printf("  server           %s\n", info.Server)
	}
	if info.DBPath != "" {
		fmt.Printf("  db-path          %s\n", info.DBPath)
	}
	fmt.Printf("  collect_interval %s\n", displayStr(info.CollectInterval, "30s"))
	fmt.Printf("  hostname         %s\n", displayStr(info.Hostname, "-"))
	fmt.Println()
}

func displayStr(s, def string) string {
	if s == "" {
		return def
	}
	return s
}

func existsMark(path string) string {
	if _, err := os.Stat(path); err == nil {
		return "found"
	}
	return "not found"
}

func displayRetention(r string) string {
	if r == "" || r == "0" {
		return "0 (forever)"
	}
	return r
}

func listPreview(items []string) string {
	if len(items) == 0 {
		return "-"
	}
	const max = 8
	parts := items
	if len(parts) > max {
		parts = parts[:max]
	}
	s := strings.Join(parts, ", ")
	if len(items) > max {
		s += ", ..."
	}
	return s
}

func humanSize(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}
