//go:build windows

package beamcore

import (
	"net"
)

// listenReusePacket on Windows binds AND joins the multicast group in one
// stdlib call. If the system holders (Dnscache/mDNS) refuse sharing, the
// bind fails and callers degrade gracefully (names unavailable, IP works).
func listenReusePacket(network, addr string) (*net.UDPConn, error) {
	raddr, err := net.ResolveUDPAddr(network, addr)
	if err != nil {
		return nil, err
	}
	return net.ListenMulticastUDP("udp4", nil, raddr)
}

// joinMulticast is a no-op on Windows: ListenMulticastUDP already joined.
func joinMulticast(conn *net.UDPConn, groupAddr string) error {
	_ = conn
	_ = groupAddr
	return nil
}
