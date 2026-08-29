package network

import (
	"testing"

	coren "github.com/yusiwen/myUtilities/internal/core/network"
)

// TestPortScanOptions_ListLocal verifies that an empty target means local mode.
func TestPortScanOptions_ListLocal(t *testing.T) {
	t.Parallel()
	opts := PortScanOptions{Ports: "8080"}
	if opts.Target != "" {
		t.Errorf("expected empty target for local mode, got %q", opts.Target)
	}
}

// TestPortScanOptions_ScanRemote verifies target is set for remote mode.
func TestPortScanOptions_ScanRemote(t *testing.T) {
	t.Parallel()
	opts := PortScanOptions{Target: "10.0.0.1", Ports: "22"}
	if opts.Target != "10.0.0.1" {
		t.Errorf("expected target 10.0.0.1, got %q", opts.Target)
	}
	if opts.Ports != "22" {
		t.Errorf("expected ports \"22\", got %q", opts.Ports)
	}
}

// TestPortScanOptions_ResolvePorts verifies port resolution via core.
func TestPortScanOptions_ResolvePorts(t *testing.T) {
	t.Parallel()

	// Explicit ports parsed through core.
	ports, err := coren.ParsePortRange("22,80")
	if err != nil {
		t.Fatalf("ParsePortRange: %v", err)
	}
	if len(ports) != 2 || ports[0] != 22 || ports[1] != 80 {
		t.Errorf("ports = %v, want [22 80]", ports)
	}

	// Common ports
	if len(coren.DefaultScanPorts) == 0 {
		t.Error("DefaultScanPorts should not be empty")
	}
}
