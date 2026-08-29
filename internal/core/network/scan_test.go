package network

import (
	"testing"
)

func TestParsePortRange_Single(t *testing.T) {
	ports, err := ParsePortRange("8080")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ports) != 1 || ports[0] != 8080 {
		t.Errorf("expected [8080], got %v", ports)
	}
}

func TestParsePortRange_Range(t *testing.T) {
	ports, err := ParsePortRange("80-82")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ports) != 3 {
		t.Fatalf("expected 3 ports, got %d: %v", len(ports), ports)
	}
	if ports[0] != 80 || ports[1] != 81 || ports[2] != 82 {
		t.Errorf("expected [80 81 82], got %v", ports)
	}
}

func TestParsePortRange_Mixed(t *testing.T) {
	ports, err := ParsePortRange("22,80,443,8000-8002")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := []int{22, 80, 443, 8000, 8001, 8002}
	if len(ports) != len(expected) {
		t.Fatalf("expected %d ports, got %d: %v", len(expected), len(ports), ports)
	}
	for i, want := range expected {
		if ports[i] != want {
			t.Errorf("ports[%d] = %d, want %d", i, ports[i], want)
		}
	}
}

func TestParsePortRange_Dedup(t *testing.T) {
	ports, err := ParsePortRange("80,80,443")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ports) != 2 {
		t.Fatalf("expected 2 unique ports, got %d: %v", len(ports), ports)
	}
}

func TestParsePortRange_Empty(t *testing.T) {
	_, err := ParsePortRange("")
	if err == nil {
		t.Error("expected error for empty spec")
	}
}

func TestParsePortRange_Invalid(t *testing.T) {
	_, err := ParsePortRange("abc")
	if err == nil {
		t.Error("expected error for non-numeric port")
	}

	_, err = ParsePortRange("80-abc")
	if err == nil {
		t.Error("expected error for range with invalid end")
	}

	_, err = ParsePortRange("abc-80")
	if err == nil {
		t.Error("expected error for range with invalid start")
	}
}

func TestParsePortRange_OutOfRange(t *testing.T) {
	_, err := ParsePortRange("0")
	if err == nil {
		t.Error("expected error for port 0")
	}

	_, err = ParsePortRange("70000")
	if err == nil {
		t.Error("expected error for port > 65535")
	}
}

func TestParseHexIP4(t *testing.T) {
	// /proc/net/tcp stores IPv4 as a little-endian 32-bit hex word:
	// 127.0.0.1 → 0x7F000001, so hexToIPPort receives "7F000001".
	ip, err := parseHexIP4("7F000001")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ip != "127.0.0.1" {
		t.Errorf("expected 127.0.0.1, got %s", ip)
	}
}

func TestParseHexIP4_Invalid(t *testing.T) {
	_, err := parseHexIP4("ZZZZ")
	if err == nil {
		t.Error("expected error for invalid hex IP")
	}
}

func TestParseHexIP6(t *testing.T) {
	// ::1 in /proc/net/tcp6: 16 bytes = 00×15 + 01, split into four 32-bit
	// words. Last word 00000001 → bytes 00 00 00 01.
	ip, err := parseHexIP6("00000000:00000000:00000000:00000001")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ip != "::1" {
		t.Errorf("expected ::1, got %s", ip)
	}
}

func TestParseHexIP6_Zero(t *testing.T) {
	// 0.0.0.0 → ::
	ip, err := parseHexIP6("00000000:00000000:00000000:00000000")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ip != "::" {
		t.Errorf("expected ::, got %s", ip)
	}
}

func TestParseHexIP6_Invalid(t *testing.T) {
	_, err := parseHexIP6("notahex")
	if err == nil {
		t.Error("expected error for invalid hex")
	}
}

func TestHexToIPPort_V4(t *testing.T) {
	// /proc/net/tcp stores IPv4 little-endian: 127.0.0.1 → "7F000001"
	ip, port, err := hexToIPPort("7F000001", "1F90", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ip != "127.0.0.1" {
		t.Errorf("ip = %q, want 127.0.0.1", ip)
	}
	if port != 8080 { // 0x1F90 = 8080
		t.Errorf("port = %d, want 8080", port)
	}
}

func TestHexToIPPort_V6(t *testing.T) {
	ip, port, err := hexToIPPort("00000000:00000000:00000000:00000001", "0050", true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ip != "::1" {
		t.Errorf("ip = %q, want ::1", ip)
	}
	if port != 80 { // 0x0050 = 80
		t.Errorf("port = %d, want 80", port)
	}
}

func TestDefaultScanPorts(t *testing.T) {
	ports := DefaultScanPorts
	if len(ports) == 0 {
		t.Error("DefaultScanPorts should not be empty")
	}
	found := make(map[int]bool)
	for _, p := range ports {
		found[p] = true
	}
	for _, expected := range []int{22, 80, 443, 3306, 5432} {
		if !found[expected] {
			t.Errorf("expected port %d in DefaultScanPorts", expected)
		}
	}
}

func TestSplitHostPort(t *testing.T) {
	cases := []struct {
		input    string
		wantHost string
		wantPort int
		wantErr  bool
	}{
		{"127.0.0.1:8080", "127.0.0.1", 8080, false},
		{"example.com:443", "example.com", 443, false},
		{"[::1]:8080", "::1", 8080, false},
		{"*:80", "0.0.0.0", 80, false},
		{"0.0.0.0:80", "0.0.0.0", 80, false},
		{":::80", "0.0.0.0", 80, false},
		{"127.0.0.1", "", 0, true},      // no port → error
		{"example.com", "", 0, true},    // no port → error
		{"[::1]:bad", "", 0, true},      // invalid port
	}
	for _, tc := range cases {
		host, port, err := splitHostPort(tc.input)
		if (err != nil) != tc.wantErr {
			t.Errorf("splitHostPort(%q) error = %v, wantErr %v", tc.input, err, tc.wantErr)
			continue
		}
		if !tc.wantErr {
			if host != tc.wantHost {
				t.Errorf("splitHostPort(%q) host = %q, want %q", tc.input, host, tc.wantHost)
			}
			if port != tc.wantPort {
				t.Errorf("splitHostPort(%q) port = %d, want %d", tc.input, port, tc.wantPort)
			}
		}
	}
}
