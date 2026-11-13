package serve

import (
	"fmt"
	"net"
	"sort"
	"strings"
)

// Addr is a local IPv4 address the server may be reachable at.
type Addr struct {
	IP        net.IP
	Interface string
	// Virtual marks addresses that usually belong to something other than the
	// physical LAN (containers, VPNs, hypervisor bridges). Such an address is
	// still listed, but a phone on the same Wi-Fi normally cannot reach it.
	Virtual bool
}

// LocalIPv4s returns every usable IPv4 address assigned to an interface that is
// up and not loopback. Link-local (169.254.0.0/16) addresses are skipped: they
// only appear when DHCP failed, and no phone can reach them reliably.
func LocalIPv4s() ([]Addr, error) {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil, fmt.Errorf("list interfaces: %w", err)
	}

	var addrs []Addr
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		ifaceAddrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, a := range ifaceAddrs {
			ipNet, ok := a.(*net.IPNet)
			if !ok {
				continue
			}
			ip4 := ipNet.IP.To4()
			if ip4 == nil || ip4.IsLoopback() || ip4.IsLinkLocalUnicast() {
				continue
			}
			addrs = append(addrs, Addr{
				IP:        ip4,
				Interface: iface.Name,
				Virtual:   isVirtualInterface(iface),
			})
		}
	}
	return addrs, nil
}

// dialUDP is a seam so tests can exercise OutboundIP without a network.
var dialUDP = func(network, address string) (net.Conn, error) {
	return net.Dial(network, address)
}

// outboundTargets are public addresses used only to ask the kernel which local
// address it would source packets from. A UDP "connection" sends no traffic, so
// this works even when the machine is offline (it then fails quickly).
var outboundTargets = []string{"8.8.8.8:80", "1.1.1.1:80"}

// OutboundIP returns the IPv4 address the operating system would use to reach
// the internet. That is normally the address a phone on the same LAN reaches,
// which makes it a far better guess than the first interface in the list.
func OutboundIP() (net.IP, error) {
	for _, target := range outboundTargets {
		conn, err := dialUDP("udp4", target)
		if err != nil {
			continue
		}
		ip, _ := conn.LocalAddr().(*net.UDPAddr)
		conn.Close()
		if ip == nil || ip.IP == nil {
			continue
		}
		ip4 := ip.IP.To4()
		if ip4 == nil || ip4.IsUnspecified() || ip4.IsLoopback() {
			continue
		}
		return ip4, nil
	}
	return nil, fmt.Errorf("no outbound IPv4 address found")
}

// Advertise returns the addresses a LAN client is most likely to reach, best
// first. An explicit interface name restricts the result to that interface.
func Advertise(ifaceName string) ([]Addr, error) {
	addrs, err := LocalIPv4s()
	if err != nil {
		return nil, err
	}

	if ifaceName != "" {
		var filtered []Addr
		for _, a := range addrs {
			if strings.EqualFold(a.Interface, ifaceName) {
				filtered = append(filtered, a)
			}
		}
		if len(filtered) == 0 {
			return nil, fmt.Errorf("interface %q has no usable IPv4 address", ifaceName)
		}
		addrs = filtered
	}

	// The outbound address is only a hint; an unreachable default route must
	// not stop the server from advertising the remaining addresses.
	preferred, _ := OutboundIP()
	orderAddrs(addrs, preferred)
	return addrs, nil
}

// orderAddrs sorts addrs in place: the outbound address first, then physical
// addresses, then virtual ones. Ties keep their original order.
func orderAddrs(addrs []Addr, preferred net.IP) {
	rank := func(a Addr) int {
		switch {
		case preferred != nil && a.IP.Equal(preferred):
			return 0
		case a.Virtual:
			return 2
		default:
			return 1
		}
	}
	sort.SliceStable(addrs, func(i, j int) bool {
		return rank(addrs[i]) < rank(addrs[j])
	})
}

// virtualPrefixes name interfaces that exist for containers, virtual machines,
// VPNs or tunnels. The list covers Linux, macOS and Windows conventions.
var virtualPrefixes = []string{
	"docker", "br-", "veth", "virbr", "vmnet", "vboxnet", "cni", "flannel",
	"podman", "colima", "utun", "tun", "tap", "tailscale", "zt", "wg",
	"nordlynx", "ham", "ppp",
}

// isVirtualInterface reports whether iface is unlikely to be a physical LAN
// interface.
func isVirtualInterface(iface net.Interface) bool {
	name := strings.ToLower(iface.Name)
	for _, prefix := range virtualPrefixes {
		if strings.HasPrefix(name, prefix) {
			return true
		}
	}
	// Point-to-point interfaces carry tunnels and VPN links. A physical Wi-Fi or
	// Ethernet interface is broadcast-capable.
	return iface.Flags&net.FlagPointToPoint != 0
}
