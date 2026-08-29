package network

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

// LocalListener describes a TCP/UDP socket in LISTEN state on the local host.
//
// Process/Command are best-effort: on Linux we read /proc/<pid>/cmdline, on
// other platforms we rely on `lsof` which already reports the command name.
// When the process owner cannot be resolved (e.g. root-owned kernel sockets),
// these fields are empty.
type LocalListener struct {
	Proto   string // "tcp" / "udp"
	LocalIP string
	Port    int
	PID     int
	User    string
	Command string
}

// ListenerOptions tunes ListLocalListeners.
type ListenerOptions struct {
	OnlyTCP bool // when true, skip UDP sockets
}

// ListLocalListeners returns every LISTEN socket on the local host.
// It picks a platform-specific backend on first call and caches it; callers
// may invoke this repeatedly (e.g. for polling).
func ListLocalListeners(opts ListenerOptions) ([]LocalListener, error) {
	backend, err := pickLocalListenerBackend()
	if err != nil {
		return nil, err
	}
	return backend(opts)
}

type listenerBackend func(ListenerOptions) ([]LocalListener, error)

var (
	localListenerOnce sync.Once
	localListenerFn   listenerBackend
	localListenerErr  error
)

func pickLocalListenerBackend() (listenerBackend, error) {
	localListenerOnce.Do(func() {
		switch runtime.GOOS {
		case "linux":
			if _, statErr := os.Stat("/proc/net/tcp"); statErr == nil {
				localListenerFn = listLinuxListeners
				return
			}
			// /proc not mounted (unusual) — fall through to lsof.
			localListenerFn = listLsofListeners
		default:
			localListenerFn = listLsofListeners
		}
		if _, execErr := exec.LookPath("lsof"); execErr != nil {
			localListenerErr = fmt.Errorf("port listing requires either /proc (Linux) or the `lsof` binary; install lsof or run on Linux: %w", execErr)
		}
	})
	if localListenerErr != nil {
		return nil, localListenerErr
	}
	return localListenerFn, nil
}

// ---- Linux backend -------------------------------------------------------

// hexToIP:4 converts the "ADDR:PORT" hex encoding used by /proc/net/tcp{,6}.
// v6 addresses are stored as four little-endian 32-bit words joined by ':'.
func hexToIPPort(addr, portHex string, isV6 bool) (string, int, error) {
	port, err := strconv.ParseInt(portHex, 16, 32)
	if err != nil {
		return "", 0, err
	}
	if !isV6 {
		ip, err := parseHexIP4(addr)
		if err != nil {
			return "", 0, err
		}
		return ip, int(port), nil
	}
	ip, err := parseHexIP6(addr)
	if err != nil {
		return "", 0, err
	}
	return ip, int(port), nil
}

func parseHexIP4(hex string) (string, error) {
	raw, err := strconv.ParseUint(hex, 16, 32)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%d.%d.%d.%d", byte(raw>>24), byte(raw>>16), byte(raw>>8), byte(raw)), nil
}

func parseHexIP6(hex string) (string, error) {
	parts := strings.Split(hex, ":")
	if len(parts) != 4 {
		return "", fmt.Errorf("unexpected IPv6 hex length %d", len(parts))
	}
	var b [16]byte
	for i, p := range parts {
		v, err := strconv.ParseUint(p, 16, 32)
		if err != nil {
			return "", err
		}
		// Each 32-bit word is stored in network (big-endian) byte order.
		b[i*4] = byte(v >> 24)
		b[i*4+1] = byte(v >> 16)
		b[i*4+2] = byte(v >> 8)
		b[i*4+3] = byte(v)
	}
	return net.IP(b[:]).String(), nil
}

// /proc/net/tcp and /proc/net/tcp6 share the same column layout:
//
//	sl local_address rem_address st ... inode
//
// State 0A == TCP LISTEN. For UDP we keep rows with state 07 (CLOSE) because
// UDP has no "listening" state — a bound socket shows up as CLOSED.
func listLinuxListeners(opts ListenerOptions) ([]LocalListener, error) {
	out, err := readLinuxProcNet("/proc/net/tcp", "tcp", false, opts.OnlyTCP)
	if err != nil {
		return nil, err
	}
	if !opts.OnlyTCP {
		udp, udpErr := readLinuxProcNet("/proc/net/udp", "udp", false, false)
		if udpErr == nil {
			out = append(out, udp...)
		}
	}
	if v6, v6Err := readLinuxProcNet("/proc/net/tcp6", "tcp", true, opts.OnlyTCP); v6Err == nil {
		out = append(out, v6...)
	}
	if !opts.OnlyTCP {
		if v6u, v6uErr := readLinuxProcNet("/proc/net/udp6", "udp", true, false); v6uErr == nil {
			out = append(out, v6u...)
		}
	}
	return out, nil
}

func readLinuxProcNet(path, proto string, isV6, onlyTCP bool) ([]LocalListener, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var listeners []LocalListener
	lines := strings.Split(string(data), "\n")
	for i, line := range lines {
		if i == 0 { // header
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 10 {
			continue
		}
		state := fields[3]
		if onlyTCP && state != "0A" {
			continue
		}
		local := fields[1]
		inode := fields[9]
		ip, port, perr := hexToIPPort(local, portLocal(local), isV6)
		if perr != nil {
			continue
		}
		l := LocalListener{Proto: proto, LocalIP: ip, Port: port}
		if pid, user, cmd := linuxInodeToProcess(inode); pid > 0 {
			l.PID = pid
			l.User = user
			l.Command = cmd
		}
		listeners = append(listeners, l)
	}
	return listeners, nil
}

// portLocal extracts the ":HEXPORT" suffix from a /proc/net address field.
func portLocal(localAddr string) string {
	idx := strings.LastIndex(localAddr, ":")
	if idx < 0 {
		return "0"
	}
	return localAddr[idx+1:]
}

// linuxInodeToProcess walks /proc/<pid>/fd/* for a socket inode and reads
// /proc/<pid>/cmdline + /proc/<pid>/stat for user. Returns 0 if not found.
func linuxInodeToProcess(inode string) (int, string, string) {
	if inode == "0" {
		return 0, "", ""
	}
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return 0, "", ""
	}
	target := "socket:[" + inode + "]"
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		pid, perr := strconv.Atoi(e.Name())
		if perr != nil {
			continue
		}
		fds, ferr := os.ReadDir("/proc/" + e.Name() + "/fd")
		if ferr != nil {
			continue
		}
		match := false
		for _, fd := range fds {
			link, lerr := os.Readlink("/proc/" + e.Name() + "/fd/" + fd.Name())
			if lerr == nil && link == target {
				match = true
				break
			}
		}
		if !match {
			continue
		}
		cmd, _ := os.ReadFile("/proc/" + e.Name() + "/cmdline")
		cmdStr := strings.ReplaceAll(string(cmd), "\x00", " ")
		cmdStr = strings.TrimSpace(cmdStr)
		if len(cmdStr) > 120 {
			cmdStr = cmdStr[:117] + "..."
		}
		user := ""
		if statData, serr := os.ReadFile("/proc/" + e.Name() + "/stat"); serr == nil {
			// /proc/<pid>/stat layout: pid (comm) state ppid pgrp ... uid ...
			// comm may contain spaces, so anchor on the LAST ')'.
			stat := string(statData)
			if rp := strings.LastIndex(stat, ") "); rp >= 0 {
				// After "pid (comm) " the next fields are:
				// state ppid pgrp session tty tpgid ...
				// uid is the 5th field after comm.
				rest := strings.Fields(stat[rp+2:]) // skip ") " itself
				if len(rest) >= 5 {
					if uid, uerr := strconv.Atoi(rest[4]); uerr == nil {
						user = uidToString(uid)
					}
				}
			}
		}
		return pid, user, cmdStr
	}
	return 0, "", ""
}

func uidToString(uid int) string {
	// /etc/passwd lookup: cheap, avoids cgo.
	data, err := os.ReadFile("/etc/passwd")
	if err != nil {
		return strconv.Itoa(uid)
	}
	for _, line := range strings.Split(string(data), "\n") {
		parts := strings.Split(line, ":")
		if len(parts) < 3 {
			continue
		}
		if parts[2] == strconv.Itoa(uid) {
			return parts[0]
		}
	}
	return strconv.Itoa(uid)
}

// ---- lsof backend (macOS/BSD/Windows fallback) ---------------------------

func listLsofListeners(opts ListenerOptions) ([]LocalListener, error) {
	args := []string{"-nP", "-iTCP"}
	if !opts.OnlyTCP {
		args = append(args, "-iUDP")
	}
	args = append(args, "-sTCP:LISTEN")
	cmd := exec.Command("lsof", args...)
	cmd.Args[0] = "lsof" // ensure resolved path is used
	out, err := cmd.Output()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			// lsof exits 1 when no matching processes; treat as empty list.
			if strings.TrimSpace(string(ee.Stderr)) == "" || strings.Contains(string(ee.Stderr), "no matching") {
				return nil, nil
			}
		}
		return nil, fmt.Errorf("lsof failed: %w", err)
	}
	return parseLsofOutput(string(out), opts.OnlyTCP), nil
}

// parseLsofOutput handles both the default lsof layout:
//
//	COMMAND   PID USER   FD TYPE DEVICE SIZE/OFF NODE NAME
//	node     1234 root   20u  IPv4 0x...      0t0  TCP *:3000 (LISTEN)
//
// and the UDP case where the NAME column reads `UDP *:53`.
func parseLsofOutput(out string, onlyTCP bool) []LocalListener {
	var out_ []LocalListener
	lines := strings.Split(out, "\n")
	for i, line := range lines {
		if i == 0 || strings.HasPrefix(line, "COMMAND") {
			continue
		}
		// Find the last token which is like "TCP 0.0.0.0:8080 (LISTEN)" or "UDP *:53".
		fields := strings.Fields(line)
		if len(fields) < 9 {
			continue
		}
		proto := fields[4] // IPv4 / IPv6
		// Skip header repeat lines.
		if proto == "TYPE" {
			continue
		}
		// NAME column is fields[8..]; find "TCP" or "UDP" token.
		protoKind, addr := "", ""
		for j := 8; j < len(fields); j++ {
			if fields[j] == "TCP" || fields[j] == "UDP" {
				protoKind = strings.ToLower(fields[j])
				if j+1 < len(fields) {
					addr = fields[j+1]
				}
				break
			}
		}
		if protoKind == "" || addr == "" {
			continue
		}
		if onlyTCP && protoKind != "tcp" {
			continue
		}
		// Strip "(LISTEN)" if present.
		addr = strings.TrimSuffix(addr, " (LISTEN)")
		ip, port, perr := splitHostPort(addr)
		if perr != nil {
			continue
		}
		l := LocalListener{Proto: protoKind, LocalIP: ip, Port: port}
		if pid, perr := strconv.Atoi(fields[1]); perr == nil {
			l.PID = pid
		}
		l.User = fields[2]
		l.Command = fields[0]
		out_ = append(out_, l)
	}
	return out_
}

// splitHostPort splits "1.2.3.4:8080", "[::1]:8080", "*:8080" into host+port.
func splitHostPort(addr string) (string, int, error) {
	// Handle IPv6 bracket form.
	if strings.HasPrefix(addr, "[") {
		end := strings.Index(addr, "]")
		if end < 0 {
			return "", 0, fmt.Errorf("bad ipv6 addr %q", addr)
		}
		host := addr[1:end]
		rest := addr[end+1:]
		if !strings.HasPrefix(rest, ":") {
			return "", 0, fmt.Errorf("bad ipv6 addr %q", addr)
		}
		port, err := strconv.Atoi(rest[1:])
		return host, port, err
	}
	// Plain form: last colon is the port separator.
	idx := strings.LastIndex(addr, ":")
	if idx < 0 {
		return "", 0, fmt.Errorf("no port in %q", addr)
	}
	host := addr[:idx]
	port, err := strconv.Atoi(addr[idx+1:])
	if err != nil {
		return "", 0, err
	}
	if host == "*" || host == "0.0.0.0" || host == "::" || host == "" {
		host = "0.0.0.0"
	}
	return host, port, nil
}

// ---- Remote TCP probe ----------------------------------------------------

// RemotePortResult is the outcome of probing a single remote port.
type RemotePortResult struct {
	Host    string
	Port    int
	Open    bool
	Elapsed time.Duration
	Err     string // human-readable reason when !Open
}

// ScanRemoteHost probes a host on the given ports using concurrent TCP
// connections. It does NOT send application-layer handshakes — an "open"
// result means the TCP SYN was accepted.
//
// concurrency controls how many dials run in parallel (capped at 128).
// timeout is per-connection; defaults to 2s when zero.
func ScanRemoteHost(host string, ports []int, concurrency int, timeout time.Duration) ([]RemotePortResult, error) {
	if concurrency <= 0 || concurrency > 128 {
		concurrency = 32
	}
	if timeout <= 0 {
		timeout = 2 * time.Second
	}
	// Resolve host once up front so a DNS failure is reported as a hard error
	// rather than N individual "dial: unknown host" lines.
	if _, err := net.DefaultResolver.LookupIP(context.Background(), "ip", host); err != nil {
		return nil, fmt.Errorf("resolve %q: %w", host, err)
	}

	// Deduplicate + sort ports for stable output.
	seen := make(map[int]bool, len(ports))
	uniq := make([]int, 0, len(ports))
	for _, p := range ports {
		if p < 1 || p > 65535 || seen[p] {
			continue
		}
		seen[p] = true
		uniq = append(uniq, p)
	}
	if len(uniq) == 0 {
		return nil, fmt.Errorf("no valid ports to scan")
	}

	results := make([]RemotePortResult, 0, len(uniq))
	jobs := make(chan int, len(uniq))
	var mu sync.Mutex
	var wg sync.WaitGroup

	for w := 0; w < concurrency; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for port := range jobs {
				target := net.JoinHostPort(host, strconv.Itoa(port))
				start := time.Now()
				conn, derr := net.DialTimeout("tcp", target, timeout)
				elapsed := time.Since(start)
				res := RemotePortResult{Host: host, Port: port, Elapsed: elapsed}
				if derr == nil {
					res.Open = true
					_ = conn.Close()
				} else {
					res.Err = derr.Error()
				}
				mu.Lock()
				results = append(results, res)
				mu.Unlock()
			}
		}()
	}
	for _, p := range uniq {
		jobs <- p
	}
	close(jobs)
	wg.Wait()

	// Stable order: by port ascending.
	sortIntsInPlace(results, func(a, b RemotePortResult) bool { return a.Port < b.Port })
	return results, nil
}

func sortIntsInPlace(s []RemotePortResult, lt func(a, b RemotePortResult) bool) {
	// Simple insertion sort; port counts in practice are <10k.
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && lt(s[j], s[j-1]); j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
}

// DefaultScanPorts is a small curated list of well-known services, used when
// the user passes `--common` to keep the scan quick but useful.
var DefaultScanPorts = []int{
	21, 22, 23, 25, 53, 80, 110, 111, 135, 139, 143, 443, 445,
	465, 587, 993, 995, 1433, 1521, 2049, 3306, 3389, 5432, 5900,
	6379, 8080, 8443, 8888, 9090, 9200, 27017,
}

// ParsePortRange accepts a single port ("8080"), an inclusive range ("80-100"),
// or a comma-separated list mixing both ("22,443,8000-8010").
func ParsePortRange(spec string) ([]int, error) {
	var out []int
	seen := make(map[int]bool)
	add := func(p int) error {
		if p < 1 || p > 65535 {
			return fmt.Errorf("port %d out of range 1-65535", p)
		}
		if !seen[p] {
			seen[p] = true
			out = append(out, p)
		}
		return nil
	}
	for _, part := range strings.Split(spec, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if lo, hi, ok := strings.Cut(part, "-"); ok {
			l, err := strconv.Atoi(strings.TrimSpace(lo))
			if err != nil {
				return nil, fmt.Errorf("bad range %q: %w", part, err)
			}
			h, err := strconv.Atoi(strings.TrimSpace(hi))
			if err != nil {
				return nil, fmt.Errorf("bad range %q: %w", part, err)
			}
			if l > h {
				l, h = h, l
			}
			for p := l; p <= h; p++ {
				if err := add(p); err != nil {
					return nil, err
				}
			}
			continue
		}
		p, err := strconv.Atoi(part)
		if err != nil {
			return nil, fmt.Errorf("bad port spec %q", part)
		}
		if err := add(p); err != nil {
			return nil, err
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no ports specified in %q", spec)
	}
	return out, nil
}
