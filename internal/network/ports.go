package network

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/morikuni/aec"
	coren "github.com/yusiwen/myUtilities/internal/core/network"
)

// PortScanOptions is the CLI surface for `mu network port-scan`.
type PortScanOptions struct {
	Target    string `arg:"" optional:"" name:"target" help:"Target host to scan (omit for local listeners)."`
	Ports     string `short:"p" name:"ports" help:"Ports to scan: single, range (8000-8100), comma-separated list. Defaults to common ports."`
	Common    bool     `short:"c" name:"common" help:"Scan common well-known ports."`
	Timeout   string   `short:"t" name:"timeout" help:"Per-port timeout (e.g. 2s, 500ms)." default:"2s"`
	Workers   int      `short:"w" name:"workers" help:"Concurrency for remote scan." default:"32"`
	UDP       bool     `short:"u" name:"udp" help:"Include UDP ports (local listing only)."`
	NoColor   bool     `short:"C" name:"no-color" help:"Disable colored output."`
	ShowAll   bool     `short:"a" name:"all" help:"Show all results including closed ports."`
	JSON      bool     `short:"J" name:"json" help:"Output as JSON."`
}

// Run handles `mu network port-scan`.
func (o *PortScanOptions) Run() error {
	if o.Target == "" {
		return o.listLocal()
	}
	return o.scanRemote()
}

func (o *PortScanOptions) listLocal() error {
	opts := coren.ListenerOptions{OnlyTCP: !o.UDP}
	listeners, err := coren.ListLocalListeners(opts)
	if err != nil {
		return err
	}

	// Filter by port range if specified.
	ports := map[int]bool{}
	if o.Ports != "" {
		list, err := coren.ParsePortRange(o.Ports)
		if err != nil {
			return err
		}
		for _, p := range list {
			ports[p] = true
		}
	} else if o.Common {
		for _, p := range coren.DefaultScanPorts {
			ports[p] = true
		}
	}

	if len(ports) > 0 {
		filtered := make([]coren.LocalListener, 0, len(listeners))
		for _, l := range listeners {
			if ports[l.Port] {
				filtered = append(filtered, l)
			}
		}
		listeners = filtered
	}

	if o.JSON {
		enc := json.NewEncoder(os.Stdout)
		return enc.Encode(listeners)
	}

	if len(listeners) == 0 {
		fmt.Println("No matching ports found.")
		return nil
	}

	// Print table header.
	header := fmt.Sprintf("%-8s %-8s %-10s %-8s %s", "PROTO", "ADDRESS", "PORT", "PID", "PROCESS")
	sep := strings.Repeat("─", 72)
	if !o.NoColor {
		fmt.Println(aec.Apply(header, aec.CyanF))
		fmt.Println(aec.Apply(sep, aec.Faint))
	} else {
		fmt.Println(header)
		fmt.Println(sep)
	}

	for _, l := range listeners {
		pidStr := "-"
		if l.PID > 0 {
			pidStr = fmt.Sprintf("%d", l.PID)
		}
		process := l.Command
		if process == "" {
			process = l.User
		}
		line := fmt.Sprintf("%-8s %-8s %-10d %-8s %s", l.Proto, l.LocalIP, l.Port, pidStr, process)
		if !o.NoColor {
			fmt.Println(aec.Apply(line, aec.GreenF))
		} else {
			fmt.Println(line)
		}
	}
	fmt.Fprintf(os.Stderr, "\n%d port(s) listening\n", len(listeners))
	return nil
}

func (o *PortScanOptions) scanRemote() error {
	var ports []int
	var err error

	switch {
	case o.Ports != "":
		ports, err = coren.ParsePortRange(o.Ports)
	case o.Common:
		ports = coren.DefaultScanPorts
	default:
		// Default: common ports.
		ports = coren.DefaultScanPorts
	}
	if err != nil {
		return err
	}

	timeout, err := time.ParseDuration(o.Timeout)
	if err != nil {
		return fmt.Errorf("invalid timeout: %w", err)
	}

	results, err := coren.ScanRemoteHost(o.Target, ports, o.Workers, timeout)
	if err != nil {
		return err
	}

	if o.JSON {
		enc := json.NewEncoder(os.Stdout)
		return enc.Encode(results)
	}

	// Show open ports.
	openCount := 0
	for _, r := range results {
		if !r.Open && !o.ShowAll {
			continue
		}
		if r.Open {
			openCount++
			line := fmt.Sprintf("PORT %-5d/%-3s  %-4s  (%.0fms)", r.Port, "tcp", "open", float64(r.Elapsed.Milliseconds()))
			if !o.NoColor {
				fmt.Println(aec.Apply(line, aec.GreenF))
			} else {
				fmt.Println(line)
			}
		} else if o.ShowAll {
			line := fmt.Sprintf("PORT %-5d/%-3s  %-4s  %s", r.Port, "tcp", "closed", r.Err)
			if !o.NoColor {
				fmt.Println(aec.Apply(line, aec.Faint))
			} else {
				fmt.Println(line)
			}
		}
	}

	if openCount == 0 {
		fmt.Println("No open ports found.")
	}
	fmt.Fprintf(os.Stderr, "\nScan complete: %d/%d ports open on %s\n", openCount, len(results), o.Target)
	return nil
}
