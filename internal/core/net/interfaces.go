package net

import (
	"net"
	"strings"
)

// InterfaceDetail contains detailed information about a network interface
type InterfaceDetail struct {
	Interface *net.Interface
	Addrs     []net.Addr
	Type      string
	Suitable  bool
	IPv4Count int
}

// GetInterfaceDetails returns detailed information about all network interfaces
func GetInterfaceDetails() ([]InterfaceDetail, error) {
	interfaces, err := net.Interfaces()
	if err != nil {
		return nil, err
	}

	details := make([]InterfaceDetail, 0, len(interfaces))
	for i := range interfaces {
		iface := &interfaces[i]
		addrs, _ := iface.Addrs()

		detail := InterfaceDetail{
			Interface: iface,
			Addrs:     addrs,
			Type:      determineInterfaceType(iface.Name),
			Suitable:  isInterfaceSuitableForWOL(iface, addrs),
			IPv4Count: countIPv4Addresses(addrs),
		}
		details = append(details, detail)
	}

	return details, nil
}

// GetInterfaceNames returns a list of all network interface names
func GetInterfaceNames() ([]string, error) {
	interfaces, err := net.Interfaces()
	if err != nil {
		return nil, err
	}

	names := make([]string, len(interfaces))
	for i, iface := range interfaces {
		names[i] = iface.Name
	}
	return names, nil
}

// determineInterfaceType determines the type of interface based on its name
func determineInterfaceType(name string) string {
	ifName := strings.ToLower(name)
	var types []string

	if strings.Contains(ifName, "ether") && !strings.Contains(ifName, "virtual") {
		types = append(types, "Wired Ethernet")
	} else if strings.Contains(ifName, "virtual") || strings.Contains(ifName, "vmware") || strings.Contains(ifName, "vbox") {
		types = append(types, "Virtual Adapter")
	} else if strings.Contains(ifName, "wi-fi") || strings.Contains(ifName, "wlan") || strings.Contains(ifName, "wireless") {
		types = append(types, "Wireless")
	} else if strings.Contains(ifName, "bluetooth") {
		types = append(types, "Bluetooth")
	} else if strings.Contains(ifName, "tunnel") {
		types = append(types, "Tunnel")
	} else if strings.Contains(ifName, "loopback") {
		types = append(types, "Loopback")
	}

	if len(types) > 0 {
		return strings.Join(types, ", ")
	}
	return ""
}

// isInterfaceSuitableForWOL determines if an interface is suitable for Wake-on-LAN
func isInterfaceSuitableForWOL(iface *net.Interface, addrs []net.Addr) bool {
	ifName := strings.ToLower(iface.Name)

	// Loopback and tunnel interfaces are never suitable
	if strings.Contains(ifName, "loopback") || strings.Contains(ifName, "tunnel") {
		return false
	}

	// Check for at least one non-loopback IPv4 address
	for _, addr := range addrs {
		ipNet, ok := addr.(*net.IPNet)
		if ok && ipNet.IP.To4() != nil && !ipNet.IP.IsLoopback() {
			return true
		}
	}
	return false
}

// countIPv4Addresses counts the number of non-loopback IPv4 addresses on an interface
func countIPv4Addresses(addrs []net.Addr) int {
	count := 0
	for _, addr := range addrs {
		ipNet, ok := addr.(*net.IPNet)
		if ok && ipNet.IP.To4() != nil && !ipNet.IP.IsLoopback() {
			count++
		}
	}
	return count
}
