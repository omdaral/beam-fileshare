package beamcore

import (
	"fmt"
	"net"
	"strings"
	"sync"
	"time"
)

const (
	mdnsName = "beam.local"

	mdnsAddr = "224.0.0.251:5353"

	mdnsTTL = 120

	// LLMNR (RFC 4795) for the bare Windows name http://beam:2004.
	llmnrAddr = "224.0.0.252:5355"

	// Short unicast TTL per RFC 4795 (mDNS uses the long one above).
	llmnrTTL = 30
)

var llmnrNames = map[string]bool{"beam": true, "b": true}

func isLLMNRName(name string) bool {
	return llmnrNames[name]
}

// StartNameDiscovery advertises the machine's LAN IPs under beam.local
// (mDNS) and beam/b (LLMNR for Windows).
// getIPs is consulted per query so address changes (hotspot
// on/off) are picked up without restart.
// Restartable: the Android wrapper stops/starts the engine in-process.
// A second Start while responders run is a no-op (no leaked socket per
// restart); StopNameDiscovery frees the sockets so the next Start rebinds
// cleanly; and loops that died on their own (bind failure with no WiFi at
// boot) clear the flag so a later Start retries instead of staying mute
// forever (the old sync.Once stayed spent after the first failure).
var (
	ndMu      sync.Mutex
	ndRunning bool
	ndAlive   int // live responder loops (started at 2 per Start)
	ndStop    chan struct{}
	mdnsConn  *net.UDPConn
	llmnrConn *net.UDPConn
)

func StartNameDiscovery(getIPs func() []string) {
	ndMu.Lock()
	if ndRunning {
		ndMu.Unlock()
		return
	}
	ndRunning = true
	ndAlive = 2
	ndStop = make(chan struct{})
	ndMu.Unlock()
	go mdnsLoop(getIPs)
	go llmnrLoop(getIPs)
}

// StopNameDiscovery halts the responder loops and frees the multicast
// sockets (phone Stop path — a stopped server must not keep advertising
// stale addresses, and the next Start must rebind, not reuse dead ones).
func StopNameDiscovery() {
	ndMu.Lock()
	if !ndRunning {
		ndMu.Unlock()
		return
	}
	ndRunning = false
	close(ndStop)
	mc, lc := mdnsConn, llmnrConn
	mdnsConn, llmnrConn = nil, nil
	ndMu.Unlock()
	if mc != nil {
		_ = mc.Close()
	}
	if lc != nil {
		_ = lc.Close()
	}
}

// loopExited marks one responder loop dead; loops that died on their own
// (bind/join failure) clear ndRunning once both are gone so a later Start
// retries. Loops stopped via StopNameDiscovery leave the flag cleared.
func loopExited() {
	ndMu.Lock()
	defer ndMu.Unlock()
	ndAlive--
	if ndAlive < 0 {
		ndAlive = 0
	}
	if ndAlive == 0 && ndStop != nil {
		select {
		case <-ndStop: // explicit stop — ndRunning already cleared
		default:
			ndRunning = false // died alone: allow a later Start to retry
		}
	}
}
func lanIPv4s(getIPs func() []string) []net.IP {
	var out []net.IP
	for _, s := range getIPs() {
		if ip := net.ParseIP(strings.TrimSpace(s)); ip != nil && ip.To4() != nil &&
			!ip.IsLoopback() && !ip.IsLinkLocalUnicast() {
			out = append(out, ip)
		}
	}
	return out
}
func mdnsLoop(getIPs func() []string) {
	defer loopExited()
	conn, err := listenReusePacket("udp4", mdnsAddr)
	if err != nil {
		fmt.Printf("  (mDNS beam.local غير متاح: %s — رابط الـ IP يعمل طبيعياً)\n", shortErr(err))
		return
	}
	ndMu.Lock()
	if !ndRunning {
		// Stopped while binding — don't leak the socket.
		ndMu.Unlock()
		_ = conn.Close()
		return
	}
	mdnsConn = conn
	stop := ndStop
	ndMu.Unlock()
	defer conn.Close()
	if err := joinMulticast(conn, mdnsAddr); err != nil {
		fmt.Printf("  (mDNS beam.local غير متاح: %s — رابط الـ IP يعمل طبيعياً)\n", shortErr(err))
		return
	}
	dst, _ := net.ResolveUDPAddr("udp4", mdnsAddr)
	buf := make([]byte, 2048)
	for {
		// Periodic deadline: lets a Stop close break the loop promptly
		// instead of blocking on Read forever with a stale socket.
		_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
		n, src, err := conn.ReadFromUDP(buf)
		if err != nil {
			select {
			case <-stop:
				return
			default:
			}
			if ne, ok := err.(net.Error); ok && ne.Timeout() {
				continue
			}
			return
		}
		_, q, ok := parseQuestion(buf[:n])
		if !ok || q.name != mdnsName {
			continue
		}
		switch q.qtype {
		case 1, 255: // A / ANY
			if ips := lanIPv4s(getIPs); len(ips) > 0 {
				_, _ = conn.WriteToUDP(answerPacket(mdnsName, ips, 0, nil, true), dst)
			}
		case 28: // AAAA: explicit NODATA so resolvers don't retry
			_, _ = conn.WriteToUDP(nodataPacket(), dst)
		}
		_ = src
	}
}

// llmnrLoop answers single-label LLMNR queries for beam/b with unicast
// replies (RFC 4795). Only Windows guests ask LLMNR in practice; other OSes
// never query it, so answering is harmless and coexists via SO_REUSEPORT
// with systemd-resolved. Any bind failure degrades gracefully: the plain
// http://beam:2004 URL just won't resolve, IPs keep working.
func llmnrLoop(getIPs func() []string) {
	defer loopExited()
	conn, err := listenReusePacket("udp4", llmnrAddr)
	if err != nil {
		fmt.Printf("  (LLMNR beam غير متاح: %s — رابط الـ IP يعمل طبيعياً)\n", shortErr(err))
		return
	}
	ndMu.Lock()
	if !ndRunning {
		// Stopped while binding — don't leak the socket.
		ndMu.Unlock()
		_ = conn.Close()
		return
	}
	llmnrConn = conn
	stop := ndStop
	ndMu.Unlock()
	defer conn.Close()
	if err := joinMulticast(conn, llmnrAddr); err != nil {
		fmt.Printf("  (LLMNR beam غير متاح: %s — رابط الـ IP يعمل طبيعياً)\n", shortErr(err))
		return
	}
	buf := make([]byte, 2048)
	for {
		// Periodic deadline: lets a Stop close break the loop promptly
		// instead of blocking on Read forever with a stale socket.
		_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
		n, src, err := conn.ReadFromUDP(buf)
		if err != nil {
			select {
			case <-stop:
				return
			default:
			}
			if ne, ok := err.(net.Error); ok && ne.Timeout() {
				continue
			}
			return
		}
		if src == nil {
			continue
		}
		if resp := llmnrResponse(buf[:n], getIPs); resp != nil {
			_, _ = conn.WriteToUDP(resp, src)
		}
	}
}

// llmnrResponse builds the unicast reply for one raw LLMNR query packet,
// or nil when the packet is not for us (wrong name/type, response packet,
// malformed, or no LAN IPs for an A query). Pure function for testability:
// the socket loop above only adds the unicast send.
func llmnrResponse(pkt []byte, getIPs func() []string) []byte {
	id, q, ok := parseQuestion(pkt)
	if !ok || !isLLMNRName(q.name) {
		return nil
	}
	// Reconstruct the raw question section to echo it back (name +
	// QTYPE + QCLASS), as LLMNR unicast replies require QDCOUNT=1.
	question := extractQuestion(pkt)
	if question == nil {
		return nil
	}
	switch q.qtype {
	case 1, 255: // A / ANY -> our LAN IPv4s
		if ips := lanIPv4s(getIPs); len(ips) > 0 {
			return answerPacket(q.name, ips, id, question, false)
		}
	case 28: // AAAA -> explicit NODATA (ANCOUNT=0) so Windows stops retrying
		return answerPacket(q.name, nil, id, question, false)
	}
	return nil
}

// extractQuestion returns the raw question section bytes (QNAME+QTYPE+QCLASS)
// of a query packet, or nil when the packet is malformed. Used to echo the
// question back in LLMNR unicast replies.
func extractQuestion(msg []byte) []byte {
	if len(msg) < 12 {
		return nil
	}
	_, off, valid := readName(msg, 12)
	if !valid || off+4 > len(msg) {
		return nil
	}
	end := off + 4
	out := make([]byte, end-12)
	copy(out, msg[12:end])
	return out
}
func shortErr(err error) string {
	s := err.Error()
	if len([]rune(s)) > 90 {
		return string([]rune(s)[:90])
	}
	return s
}
