package downloader

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// Sentinel errors surfaced to the CLI layer.
var (
	// ErrInterrupted reports that the transfer was cancelled by the caller
	// (Ctrl-C or a cancelled context) before it finished.
	ErrInterrupted = errors.New("download interrupted")

	// ErrChecksumMismatch reports that the finished file did not match the
	// expected SHA-256 checksum.
	ErrChecksumMismatch = errors.New("checksum mismatch")

	// ErrOutputExists reports that the output file already exists and --force
	// was not given.
	ErrOutputExists = errors.New("output file already exists")

	// ErrResumeMismatch reports that the remote file changed and resuming is
	// not safe without --force-resume.
	ErrResumeMismatch = errors.New("resume state does not match the remote file")

	// errRangeIgnored is internal: the server answered a ranged request with a
	// full 200 response, so the run must fall back to single-stream mode.
	errRangeIgnored = errors.New("server ignored the range request")
)

// ParseSize parses a byte size such as "8M", "1MiB", "512k" or "1048576".
// Both SI (kB/MB/GB, 1000-based) and IEC (KiB/MiB/GiB, 1024-based) suffixes are
// accepted; a bare number is bytes.
func ParseSize(s string) (int64, error) {
	t := strings.TrimSpace(s)
	if t == "" {
		return 0, errors.New("empty size")
	}
	lower := strings.ToLower(t)

	units := []struct {
		suffix string
		mult   float64
	}{
		{"kib", 1 << 10}, {"mib", 1 << 20}, {"gib", 1 << 30}, {"tib", 1 << 40},
		{"kb", 1e3}, {"mb", 1e6}, {"gb", 1e9}, {"tb", 1e12},
		{"k", 1 << 10}, {"m", 1 << 20}, {"g", 1 << 30}, {"t", 1 << 40},
		{"b", 1},
	}

	mult := float64(1)
	num := lower
	for _, u := range units {
		if strings.HasSuffix(lower, u.suffix) {
			mult = u.mult
			num = strings.TrimSpace(strings.TrimSuffix(lower, u.suffix))
			break
		}
	}
	v, err := strconv.ParseFloat(num, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid size %q", s)
	}
	if v < 0 {
		return 0, fmt.Errorf("invalid size %q: must not be negative", s)
	}
	return int64(v * mult), nil
}

// FormatSize renders a byte count in IEC units (B, KiB, MiB, GiB, TiB).
func FormatSize(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	units := []string{"KiB", "MiB", "GiB", "TiB", "PiB"}
	value := float64(n)
	idx := -1
	for value >= unit && idx < len(units)-1 {
		value /= unit
		idx++
	}
	return fmt.Sprintf("%.1f %s", value, units[idx])
}
