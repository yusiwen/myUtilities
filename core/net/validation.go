package net

import "regexp"

var (
	// HostnameRE is a regular expression for validating hostnames
	HostnameRE = regexp.MustCompile(`^[a-zA-Z0-9]([a-zA-Z0-9\-]{0,61}[a-zA-Z0-9])?(\.[a-zA-Z0-9]([a-zA-Z0-9\-]{0,61}[a-zA-Z0-9])?)*$`)
	// MacRE is a regular expression for validating MAC addresses (aa:bb:cc:dd:ee:ff format)
	MacRE = regexp.MustCompile(`^([0-9A-Fa-f]{2}[:]){5}([0-9A-Fa-f]{2})$`)
)

// ValidHostname validates a hostname string
func ValidHostname(name string) bool {
	if len(name) > 253 {
		return false
	}
	return HostnameRE.MatchString(name)
}

// ValidMAC validates a MAC address string (aa:bb:cc:dd:ee:ff format)
func ValidMAC(mac string) bool {
	return MacRE.MatchString(mac)
}
