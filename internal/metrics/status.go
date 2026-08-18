package metrics

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	coremetrics "github.com/yusiwen/myUtilities/internal/core/metrics"
)

func (o *StatusOptions) Run() error {
	cfg, err := coremetrics.LoadConfigDir(o.ConfigDir)
	if err != nil {
		return err
	}

	cfgPath := ""
	if o.ConfigDir != "" {
		cfgPath = filepath.Join(coremetrics.ExpandTilde(o.ConfigDir), "metrics-config.json")
	} else {
		cfgPath, _ = coremetrics.DefaultConfigPath()
	}

	retention := coremetrics.ParseRetention(cfg.Retention)
	compactInterval := coremetrics.ResolveCompactInterval(cfg.CompactInterval)
	interval := coremetrics.ResolveInterval("", cfg.Interval)
	hostname := coremetrics.ResolveHostname("", cfg.Hostname)
	dbPath, err := coremetrics.ResolveDBPath(o.ConfigDir, cfg.DataDir, o.DBPath)
	if err != nil {
		return err
	}

	fmt.Println("Config:")
	fmt.Printf("  config-dir       %s\n", displayDir(o.ConfigDir))
	fmt.Printf("  config file      %s (%s)\n", cfgPath, existsMark(cfgPath))
	fmt.Printf("  retention        %s\n", displayRetention(retention))
	fmt.Printf("  compact_interval %s\n", displayRetention(compactInterval))
	fmt.Printf("  collect_interval %s\n", interval)
	fmt.Printf("  hostname         %s\n", hostname)
	if cfg.ServerURL != "" {
		fmt.Printf("  server_url       %s\n", cfg.ServerURL)
	} else {
		fmt.Printf("  server_url       (none)\n")
	}
	fmt.Printf("  db-path          %s\n", dbPath)
	fmt.Printf("  debug            %v\n", cfg.DebugLog)
	fmt.Println()

	// Running state: remote server or the local port.
	baseURL := fmt.Sprintf("http://localhost:%d", o.Port)
	if o.Server != "" {
		baseURL = strings.TrimRight(o.Server, "/")
	}
	names, namesErr := coremetrics.FetchMetricNames(baseURL)
	hosts, _ := coremetrics.FetchHosts(baseURL)
	running := namesErr == nil

	fmt.Println("Running:")
	fmt.Printf("  server           %s\n", baseURL)
	if running {
		fmt.Printf("  state            running (%d metrics)\n", len(names))
	} else {
		fmt.Printf("  state            not running\n")
	}
	fmt.Println()

	// DB stats: file stat always; counts via read-only open when the local
	// server is not running. Remote mode skips the local file entirely.
	fmt.Println("DB:")
	remote := o.Server != ""
	if !remote {
		if st, statErr := os.Stat(dbPath); statErr == nil {
			fmt.Printf("  file             %s\n", dbPath)
			fmt.Printf("  size             %s\n", humanSize(st.Size()))
			fmt.Printf("  modified         %s\n", st.ModTime().Format("2006-01-02 15:04:05 MST"))
		} else {
			fmt.Printf("  file             %s (not found)\n", dbPath)
		}
	}

	switch {
	case !remote && !running:
		ro, openErr := coremetrics.OpenReadOnly(dbPath)
		if openErr != nil {
			fmt.Printf("  note             cannot open read-only: %v\n", openErr)
		} else {
			defer ro.Close()
			stats, statsErr := ro.Stats()
			dbHosts, _ := ro.ListHosts()
			metricNames, _ := ro.ListMetricNames()
			if statsErr != nil {
				fmt.Printf("  note             stats error: %v\n", statsErr)
			} else {
				fmt.Printf("  series           %d\n", stats.Series)
				fmt.Printf("  points           %d\n", stats.Points)
			}
			fmt.Printf("  hosts            %s\n", listPreview(dbHosts))
			fmt.Printf("  metrics          %s (%d total)\n", listPreview(metricNames), len(metricNames))
		}
	case running:
		if remote {
			fmt.Printf("  series           (remote server; counts via HTTP)\n")
		} else {
			fmt.Printf("  series           (locked by running server; counts via HTTP)\n")
		}
		fmt.Printf("  hosts            %s\n", listPreview(hosts))
		fmt.Printf("  metrics          %s (%d total)\n", listPreview(names), len(names))
	case remote:
		fmt.Printf("  note             remote server unreachable; local DB not inspected\n")
	}

	return nil
}

func displayDir(dir string) string {
	if dir == "" {
		return "(default ~/.config/mu)"
	}
	return coremetrics.ExpandTilde(dir)
}

func existsMark(path string) string {
	if _, err := os.Stat(path); err == nil {
		return "found"
	}
	return "not found"
}

func displayRetention(r time.Duration) string {
	if r <= 0 {
		return "0 (forever)"
	}
	if r >= 24*time.Hour && r%(24*time.Hour) == 0 {
		return fmt.Sprintf("%dd", r/(24*time.Hour))
	}
	if r >= time.Hour && r%time.Hour == 0 {
		return fmt.Sprintf("%dh", r/time.Hour)
	}
	if r >= time.Minute && r%time.Minute == 0 {
		return fmt.Sprintf("%dm", r/time.Minute)
	}
	return r.String()
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
		s += fmt.Sprintf(", ...")
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
