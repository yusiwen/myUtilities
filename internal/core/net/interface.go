package net

import (
	"fmt"
	"net"
	"strings"
)

// GetInterfaceByName finds a network interface by name with case-insensitive fallback.
// Returns nil and an error if the interface is not found.
func GetInterfaceByName(name string) (*net.Interface, error) {
	// First try exact match
	iface, err := net.InterfaceByName(name)
	if err == nil {
		return iface, nil
	}

	// If not found, try case-insensitive search
	interfaces, err := net.Interfaces()
	if err != nil {
		return nil, fmt.Errorf("failed to list interfaces: %v", err)
	}

	lowerName := strings.ToLower(name)
	for i := range interfaces {
		if strings.ToLower(interfaces[i].Name) == lowerName {
			return &interfaces[i], nil
		}
	}

	return nil, fmt.Errorf("interface %q not found", name)
}

// SelectBestInterfaceForWOL selects the most suitable interface for WOL broadcasts.
func SelectBestInterfaceForWOL() (*net.Interface, error) {
	interfaces, err := net.Interfaces()
	if err != nil {
		return nil, fmt.Errorf("failed to list interfaces: %v", err)
	}

	var candidates []*net.Interface

	for i := range interfaces {
		iface := &interfaces[i]

		// Skip virtual, bluetooth, tunnel, and loopback interfaces
		ifName := strings.ToLower(iface.Name)
		if strings.Contains(ifName, "virtual") ||
			strings.Contains(ifName, "vmware") ||
			strings.Contains(ifName, "vbox") ||
			strings.Contains(ifName, "bluetooth") ||
			strings.Contains(ifName, "tunnel") ||
			strings.Contains(ifName, "loopback") ||
			strings.Contains(ifName, "pseudo") {
			continue
		}

		// Check for IPv4 address
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}

		hasIPv4 := false
		for _, addr := range addrs {
			ipNet, ok := addr.(*net.IPNet)
			if ok && ipNet.IP.To4() != nil && !ipNet.IP.IsLoopback() {
				hasIPv4 = true
				break
			}
		}

		if hasIPv4 {
			candidates = append(candidates, iface)
		}
	}

	// Prioritize wired Ethernet interfaces
	for _, iface := range candidates {
		ifName := strings.ToLower(iface.Name)
		if strings.Contains(ifName, "ether") && !strings.Contains(ifName, "virtual") {
			return iface, nil
		}
	}

	// Then wireless interfaces
	for _, iface := range candidates {
		ifName := strings.ToLower(iface.Name)
		if strings.Contains(ifName, "wi-fi") || strings.Contains(ifName, "wlan") || strings.Contains(ifName, "wireless") {
			return iface, nil
		}
	}

	// Return first candidate if any
	if len(candidates) > 0 {
		return candidates[0], nil
	}

	return nil, fmt.Errorf("no suitable interface found for WOL (no interfaces with IPv4 addresses)")
}

// IPFromInterface returns a *net.UDPAddr from a network interface name.
// If iface is empty, it auto-selects the best interface for WOL.
func IPFromInterface(iface string) (*net.UDPAddr, error) {
	var ief *net.Interface
	var err error

	if iface == "" {
		// Auto-select best interface
		ief, err = SelectBestInterfaceForWOL()
		if err != nil {
			return nil, fmt.Errorf("failed to auto-select interface: %v", err)
		}
		// Logging removed - should be handled by caller
	} else {
		ief, err = GetInterfaceByName(iface)
		if err != nil {
			return nil, err
		}
	}

	addrs, err := ief.Addrs()
	if err == nil && len(addrs) <= 0 {
		err = fmt.Errorf("no address associated with interface %s", ief.Name)
	}
	if err != nil {
		return nil, err
	}

	// Validate that one of the addrs is a valid network IP address.
	for _, addr := range addrs {
		switch ip := addr.(type) {
		case *net.IPNet:
			if ip.IP.To4() != nil && !ip.IP.IsLoopback() {
				return &net.UDPAddr{
					IP: ip.IP,
				}, nil
			}
		}
	}
	return nil, fmt.Errorf("no IPv4 address associated with interface %s", ief.Name)
}

// GetOutboundMAC returns the MAC address of the interface used to route
// packets to the given server address (host:port).
func GetOutboundMAC(server string) (net.HardwareAddr, error) {
	conn, err := net.Dial("udp", server)
	if err != nil {
		return nil, fmt.Errorf("failed to route to %s: %v", server, err)
	}
	defer conn.Close()

	localAddr := conn.LocalAddr().(*net.UDPAddr)

	interfaces, err := net.Interfaces()
	if err != nil {
		return nil, fmt.Errorf("failed to list interfaces: %v", err)
	}

	for _, iface := range interfaces {
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			ipNet, ok := addr.(*net.IPNet)
			if ok && ipNet.IP.Equal(localAddr.IP) {
				return iface.HardwareAddr, nil
			}
		}
	}

	return nil, fmt.Errorf("no interface found with source IP %s", localAddr.IP)
}
