package beamcore

import (
	"encoding/binary"
	"fmt"
	"net"
	"strings"
	"time"
)

const (
	mdnsName = "beam.local"

	mdnsAddr = "224.0.0.251:5353"

	mdnsTTL = 120

	// LLMNR (RFC 4795) for the bare Windows name http://beam:2004.
var llmnrNames = map[string]bool{"beam": true, "b": true}
func isLLMNRName(name string) bool {
	return llmnrNames[name]
}
// StartNameDiscovery advertises the machine's LAN IPs under beam.local
// (mDNS) and beam/b (LLMNR for Windows).
// getIPs is consulted per query so address changes (hotspot
// on/off) are picked up without restart.
func StartNameDiscovery(getIPs func() []string) {
	go mdnsLoop(getIPs)
	go llmnrLoop(getIPs)
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
	conn, err := listenReusePacket("udp4", mdnsAddr)
	if err != nil {
		fmt.Printf("  (mDNS beam.local غير متاح: %s — رابط الـ IP يعمل طبيعياً)\n", shortErr(err))
		return
	}
	defer conn.Close()
	if err := joinMulticast(conn, mdnsAddr); err != nil {
		fmt.Printf("  (mDNS beam.local غير متاح: %s — رابط الـ IP يعمل طبيعياً)\n", shortErr(err))
		return
	}
	dst, _ := net.ResolveUDPAddr("udp4", mdnsAddr)
	buf := make([]byte, 2048)
	for {
		n, src, err := conn.ReadFromUDP(buf)
		if err != nil {
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
	conn, err := listenReusePacket("udp4", llmnrAddr)
	if err != nil {
		fmt.Printf("  (LLMNR beam غير متاح: %s — رابط الـ IP يعمل طبيعياً)\n", shortErr(err))
		return
	}
	defer conn.Close()
	if err := joinMulticast(conn, llmnrAddr); err != nil {
		fmt.Printf("  (LLMNR beam غير متاح: %s — رابط الـ IP يعمل طبيعياً)\n", shortErr(err))
		return
	}
	buf := make([]byte, 2048)
	for {
		n, src, err := conn.ReadFromUDP(buf)
		if err != nil {
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
