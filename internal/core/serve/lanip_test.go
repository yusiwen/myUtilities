package serve

import (
	"net"
	"testing"
	"time"
)

// fakeConn is a net.Conn that only reports a local address, which is all
// OutboundIP reads.
type fakeConn struct {
	local net.Addr
}

func (c *fakeConn) Read([]byte) (int, error)         { return 0, nil }
func (c *fakeConn) Write(b []byte) (int, error)      { return len(b), nil }
func (c *fakeConn) Close() error                     { return nil }
func (c *fakeConn) LocalAddr() net.Addr              { return c.local }
func (c *fakeConn) RemoteAddr() net.Addr             { return &net.UDPAddr{IP: net.IPv4(8, 8, 8, 8)} }
func (c *fakeConn) SetDeadline(time.Time) error      { return nil }
func (c *fakeConn) SetReadDeadline(time.Time) error  { return nil }
func (c *fakeConn) SetWriteDeadline(time.Time) error { return nil }

func withDialUDP(t *testing.T, fn func(network, address string) (net.Conn, error)) {
	t.Helper()
	original := dialUDP
	dialUDP = fn
	t.Cleanup(func() { dialUDP = original })
}

func TestOutboundIPUsesLocalAddress(t *testing.T) {
	withDialUDP(t, func(network, address string) (net.Conn, error) {
		if network != "udp4" {
			t.Errorf("dialed network %q, want udp4", network)
		}
		return &fakeConn{local: &net.UDPAddr{IP: net.IPv4(192, 168, 1, 20), Port: 53535}}, nil
	})

	ip, err := OutboundIP()
	if err != nil {
		t.Fatalf("OutboundIP: %v", err)
	}
	if !ip.Equal(net.IPv4(192, 168, 1, 20)) {
		t.Errorf("OutboundIP = %v, want 192.168.1.20", ip)
	}
}

func TestOutboundIPRejectsLoopback(t *testing.T) {
	withDialUDP(t, func(network, address string) (net.Conn, error) {
		return &fakeConn{local: &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)}}, nil
	})

	if ip, err := OutboundIP(); err == nil {
		t.Fatalf("OutboundIP = %v, want an error for a loopback source address", ip)
	}
}

func TestOutboundIPFallsBackToSecondTarget(t *testing.T) {
	var calls int
	withDialUDP(t, func(network, address string) (net.Conn, error) {
		calls++
		if calls == 1 {
			return nil, &net.OpError{Op: "dial", Err: net.ErrClosed}
		}
		return &fakeConn{local: &net.UDPAddr{IP: net.IPv4(10, 0, 0, 7)}}, nil
	})

	ip, err := OutboundIP()
	if err != nil {
		t.Fatalf("OutboundIP: %v", err)
	}
	if !ip.Equal(net.IPv4(10, 0, 0, 7)) {
		t.Errorf("OutboundIP = %v, want 10.0.0.7", ip)
	}
}

func TestOrderAddrsPutsOutboundFirstThenPhysical(t *testing.T) {
	addrs := []Addr{
		{IP: net.IPv4(172, 17, 0, 1), Interface: "docker0", Virtual: true},
		{IP: net.IPv4(192, 168, 1, 30), Interface: "en1"},
		{IP: net.IPv4(192, 168, 1, 20), Interface: "en0"},
	}
	orderAddrs(addrs, net.IPv4(192, 168, 1, 20))

	want := []string{"192.168.1.20", "192.168.1.30", "172.17.0.1"}
	for i, w := range want {
		if got := addrs[i].IP.String(); got != w {
			t.Fatalf("position %d = %s, want %s (full order: %v)", i, got, w, ipStrings(addrs))
		}
	}
}

func TestOrderAddrsWithoutOutboundHintKeepsPhysicalFirst(t *testing.T) {
	addrs := []Addr{
		{IP: net.IPv4(10, 8, 0, 2), Interface: "utun3", Virtual: true},
		{IP: net.IPv4(192, 168, 0, 42), Interface: "en0"},
	}
	orderAddrs(addrs, nil)

	if got := addrs[0].IP.String(); got != "192.168.0.42" {
		t.Errorf("first address = %s, want the physical 192.168.0.42", got)
	}
}

func TestAdvertiseRejectsUnknownInterface(t *testing.T) {
	if _, err := Advertise("definitely-not-an-interface"); err == nil {
		t.Fatal("Advertise accepted an interface that does not exist")
	}
}

func TestIsVirtualInterface(t *testing.T) {
	cases := []struct {
		name string
		want bool
	}{
		{"en0", false},
		{"eth0", false},
		{"Wi-Fi", false},
		{"docker0", true},
		{"br-1a2b3c", true},
		{"utun3", true},
		{"tailscale0", true},
		{"veth12ab", true},
	}
	for _, c := range cases {
		if got := isVirtualInterface(net.Interface{Name: c.name}); got != c.want {
			t.Errorf("isVirtualInterface(%q) = %v, want %v", c.name, got, c.want)
		}
	}
	// Point-to-point flags mark tunnels even when the name looks ordinary.
	ptp := net.Interface{Name: "ppp0", Flags: net.FlagUp | net.FlagPointToPoint}
	if !isVirtualInterface(ptp) {
		t.Error("point-to-point interface not treated as virtual")
	}
}

func ipStrings(addrs []Addr) []string {
	out := make([]string, len(addrs))
	for i, a := range addrs {
		out[i] = a.IP.String()
	}
	return out
}
