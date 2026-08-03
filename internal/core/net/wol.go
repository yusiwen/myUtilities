package net

import (
	"fmt"
	"net"

	"github.com/sabhiram/go-wol/wol"
)

// SendWOL sends a Wake-on-LAN magic packet to the specified MAC address.
// The iface parameter specifies which network interface to use (empty for auto-select).
func SendWOL(mac, iface string) error {
	// Default broadcast address and port
	bcastIP := "255.255.255.255"
	udpPort := "9"

	// Determine local address based on interface
	localAddr, err := IPFromInterface(iface)
	if err != nil {
		return err
	}

	// Resolve broadcast address
	bcastAddr := fmt.Sprintf("%s:%s", bcastIP, udpPort)
	udpAddr, err := net.ResolveUDPAddr("udp", bcastAddr)
	if err != nil {
		return err
	}

	// Build magic packet
	mp, err := wol.New(mac)
	if err != nil {
		return err
	}

	// Marshal to bytes
	bs, err := mp.Marshal()
	if err != nil {
		return err
	}

	// Create UDP connection
	conn, err := net.DialUDP("udp", localAddr, udpAddr)
	if err != nil {
		return err
	}
	defer conn.Close()

	// Send packet
	n, err := conn.Write(bs)
	if err != nil {
		return err
	}
	if n != 102 {
		return fmt.Errorf("magic packet sent was %d bytes (expected 102 bytes sent)", n)
	}
	return nil
}
