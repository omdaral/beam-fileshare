package beamcore

import (
	"encoding/binary"
	"net"
	"strings"
)

type dnsQuestion struct {
	name  string // lowercased, no trailing dot
	qtype uint16
}
// parseQuestion reads the first question of a DNS message.
// Returns false for responses, malformed packets, or zero questions.
func parseQuestion(msg []byte) (id uint16, q dnsQuestion, ok bool) {
	if len(msg) < 12 {
		return 0, q, false
	}
	flags := binary.BigEndian.Uint16(msg[2:4])
	if flags&0x8000 != 0 { // QR: incoming response, ignore
		return 0, q, false
	}
	if binary.BigEndian.Uint16(msg[4:6]) < 1 {
		return 0, q, false
	}
	name, off, valid := readName(msg, 12)
	if !valid || off+4 > len(msg) {
		return 0, q, false
	}
	return binary.BigEndian.Uint16(msg[0:2]),
		dnsQuestion{name: name, qtype: binary.BigEndian.Uint16(msg[off : off+2])},
		true
}
// readName decodes a possibly compressed domain name.
func readName(msg []byte, off int) (string, int, bool) {
	var parts []string
	visited := map[int]bool{}
	jumped := false
	end := off
	for i := 0; i < 64; i++ { // loop guard
		if off >= len(msg) || visited[off] {
			return "", 0, false
		}
		visited[off] = true
		b := msg[off]
		if b&0xC0 == 0xC0 { // compression pointer
			if off+1 >= len(msg) {
				return "", 0, false
			}
			ptr := int(binary.BigEndian.Uint16(msg[off:off+2]) & 0x3FFF)
			if !jumped {
				end = off + 2
			}
			off = ptr
			jumped = true
			continue
		}
		if b == 0 {
			if !jumped {
				end = off + 1
			}
			return strings.Join(parts, "."), end, true
		}
		if b&0xC0 != 0 || off+1+int(b) > len(msg) {
			return "", 0, false
		}
		parts = append(parts, strings.ToLower(string(msg[off+1:off+1+int(b)])))
		off += 1 + int(b)
		if !jumped {
			end = off
		}
	}
	return "", 0, false // exhausted without terminator
}
func encodeName(name string) []byte {
	var out []byte
	for _, p := range strings.Split(strings.TrimSuffix(name, "."), ".") {
		if len(p) > 63 {
			p = p[:63]
		}
		out = append(out, byte(len(p)))
		out = append(out, p...)
	}
	return append(out, 0)
}
// answerPacket builds an mDNS/LLMNR response advertising ips as A records.
// When mdns is true: ID 0, no question echo (multicast announce), class
// WITH cache-flush bit (0x8001) and mDNS TTL.
// Otherwise (LLMNR unicast): ID echoed + question echoed back (QDCOUNT=1),
// plain IN class (1) and short LLMNR TTL. The header byte 0x84 doubles as
// QR+AA (DNS) and QR+C (LLMNR RFC 4795: C lives where AA lives).
func answerPacket(name string, ips []net.IP, id uint16, question []byte, mdns bool) []byte {
	hdr := make([]byte, 12)
	ttl := uint32(llmnrTTL)
	class := uint16(1)
	if mdns {
		hdr[2] = 0x84 // QR + AA
		ttl = uint32(mdnsTTL)
		class = uint16(1 | 0x8000) // cache-flush
	} else {
		binary.BigEndian.PutUint16(hdr[0:2], id)
		hdr[2] = 0x84 // QR + C (unique)
		if question != nil {
			binary.BigEndian.PutUint16(hdr[4:6], 1)
		}
	}
	msg := append([]byte{}, hdr...)
	if !mdns && question != nil {
		msg = append(msg, question...)
	}
	n := 0
	for _, ip := range ips {
		v4 := ip.To4()
		if v4 == nil {
			continue
		}
		msg = append(msg, encodeName(name)...)
		rr := make([]byte, 10)
		binary.BigEndian.PutUint16(rr[0:2], 1) // A
		binary.BigEndian.PutUint16(rr[2:4], class)
		binary.BigEndian.PutUint32(rr[4:8], ttl)
		binary.BigEndian.PutUint16(rr[8:10], 4)
		msg = append(msg, rr...)
		msg = append(msg, v4...)
		n++
	}
	binary.BigEndian.PutUint16(msg[6:8], uint16(n))
	return msg
}
// nodataPacket answers AAAA-with-no-data (stops resolver retries).
func nodataPacket() []byte {
	msg := make([]byte, 12)
	msg[2] = 0x84 // QR + AA
	return msg
}
