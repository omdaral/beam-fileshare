//go:build !windows

package beamcore

import (
	"net"
	"runtime"
	"syscall"
)

// soReusePort is SO_REUSEPORT's numeric value (absent from stdlib syscall
// on Linux; present on darwin with a different value).
func soReusePort() int {
	if runtime.GOOS == "darwin" {
		return 0x200
	}
	return 15 // Linux and other unixes sharing the mDNS/LLMNR ports
}

// listenReusePacket binds UDP with SO_REUSEADDR+SO_REUSEPORT (stdlib only)
// so Beam coexists with avahi/systemd-resolved on the shared mDNS/LLMNR ports.
func listenReusePacket(network, addr string) (*net.UDPConn, error) {
	raddr, err := net.ResolveUDPAddr(network, addr)
	if err != nil {
		return nil, err
	}
	lc := net.ListenConfig{
		Control: func(_, _ string, c syscall.RawConn) error {
			var opErr error
			ctrlErr := c.Control(func(fd uintptr) {
				opErr = syscall.SetsockoptInt(int(fd), syscall.SOL_SOCKET, syscall.SO_REUSEADDR, 1)
				if opErr != nil {
					return
				}
				opErr = syscall.SetsockoptInt(int(fd), syscall.SOL_SOCKET, soReusePort(), 1)
			})
			if ctrlErr != nil {
				return ctrlErr
			}
			return opErr
		},
	}
	p, err := lc.ListenPacket(nil, network, raddr.String())
	if err != nil {
		return nil, err
	}
	conn, ok := p.(*net.UDPConn)
	if !ok {
		_ = p.Close()
		return nil, syscall.EINVAL
	}
	return conn, nil
}

// joinMulticast joins the group on every multicast-capable IPv4 interface.
func joinMulticast(conn *net.UDPConn, groupAddr string) error {
	gaddr, err := net.ResolveUDPAddr("udp4", groupAddr)
	if err != nil {
		return err
	}
	ifaces, err := net.Interfaces()
	if err != nil || len(ifaces) == 0 {
		return connJoin(conn, gaddr.IP, nil)
	}
	joined := false
	for _, ifi := range ifaces {
		if ifi.Flags&net.FlagUp == 0 || ifi.Flags&net.FlagMulticast == 0 {
			continue
		}
		if err := connJoin(conn, gaddr.IP, &ifi); err == nil {
			joined = true
		}
	}
	if !joined {
		return connJoin(conn, gaddr.IP, nil)
	}
	return nil
}

func connJoin(conn *net.UDPConn, group net.IP, ifi *net.Interface) error {
	rc, err := conn.SyscallConn()
	if err != nil {
		return err
	}
	var opErr error
	ctrlErr := rc.Control(func(fd uintptr) {
		mreq := &syscall.IPMreq{Multiaddr: [4]byte{}}
		copy(mreq.Multiaddr[:], group.To4())
		if ifi != nil {
			if addrs, err := ifi.Addrs(); err == nil {
				for _, a := range addrs {
					ip, _, _ := net.ParseCIDR(a.String())
					if ip == nil {
						ip = net.ParseIP(a.String())
					}
					if ip != nil && ip.To4() != nil && !ip.IsLoopback() {
						copy(mreq.Interface[:], ip.To4())
						break
					}
				}
			}
		}
		opErr = syscall.SetsockoptIPMreq(int(fd), syscall.IPPROTO_IP, syscall.IP_ADD_MEMBERSHIP, mreq)
	})
	if ctrlErr != nil {
		return ctrlErr
	}
	return opErr
}
