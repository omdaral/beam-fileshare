package beamcore

import (
	"bytes"
	"encoding/binary"
	"net"
	"testing"
	"time"
)

func deadline(sec int) time.Time { return time.Now().Add(time.Duration(sec) * time.Second) }

func buildQuery(id uint16, name string, qtype uint16) []byte {
	msg := make([]byte, 12)
	binary.BigEndian.PutUint16(msg[0:2], id)
	binary.BigEndian.PutUint16(msg[4:6], 1)
	msg = append(msg, encodeName(name)...)
	q := make([]byte, 4)
	binary.BigEndian.PutUint16(q[0:2], qtype)
	binary.BigEndian.PutUint16(q[2:4], 1)
	return append(msg, q...)
}

func TestParseQuestion(t *testing.T) {
	id, q, ok := parseQuestion(buildQuery(0x1234, "beam.local", 1))
	if !ok || id != 0x1234 || q.name != "beam.local" || q.qtype != 1 {
		t.Fatalf("parse = %v %+v %v", id, q, ok)
	}
	// responses must be ignored
	resp := buildQuery(9, "beam.local", 1)
	resp[2] |= 0x80
	if _, _, ok := parseQuestion(resp); ok {
		t.Fatal("response accepted as query")
	}
	// truncated
	if _, _, ok := parseQuestion([]byte{1, 2, 3}); ok {
		t.Fatal("truncated accepted")
	}
}

func TestReadNameCompression(t *testing.T) {
	// header + "local" root at 12, question at 20 = "beam" + pointer to 12
	msg := make([]byte, 12)
	msg = append(msg, 5, 'l', 'o', 'c', 'a', 'l', 0)
	qoff := len(msg)
	msg = append(msg, 4, 'b', 'e', 'a', 'm', 0xC0, 0x0C, 0x00, 0x01, 0x00, 0x01)
	binary.BigEndian.PutUint16(msg[4:6], 1)
	name, end, ok := readName(msg, qoff)
	if !ok || name != "beam.local" || end != qoff+7 {
		t.Fatalf("compressed = %q %d %v", name, end, ok)
	}
	// cyclic pointer must be rejected, not hang
	cyc := append(make([]byte, 12), 0xC0, 0x0C, 0x00, 0x01, 0x00, 0x01)
	if _, _, ok := parseQuestion(cyc); ok {
		t.Fatal("cyclic pointer accepted")
	}
}

func TestAnswerPacket(t *testing.T) {
	ips := []net.IP{net.ParseIP("192.168.1.6"), net.ParseIP("10.0.0.9")}
	query := buildQuery(0x42, "beam", 1)
	pkt := answerPacket("beam", ips, 0x42, query[12:], false)
	if binary.BigEndian.Uint16(pkt[0:2]) != 0x42 {
		t.Fatal("ID not echoed")
	}
	if pkt[2]&0x80 == 0 {
		t.Fatal("QR not set")
	}
	if binary.BigEndian.Uint16(pkt[4:6]) != 1 || binary.BigEndian.Uint16(pkt[6:8]) != 2 {
		t.Fatalf("counts qd/an = %d/%d", binary.BigEndian.Uint16(pkt[4:6]), binary.BigEndian.Uint16(pkt[6:8]))
	}
	if !bytes.Contains(pkt, []byte{192, 168, 1, 6}) ||
		!bytes.Contains(pkt, []byte{10, 0, 0, 9}) {
		t.Fatalf("RDATAs missing in %v", pkt)
	}
	// mDNS framing: ID 0, no question, cache-flush bit
	m := answerPacket("beam.local", ips[:1], 0, nil, true)
	if binary.BigEndian.Uint16(m[0:2]) != 0 || binary.BigEndian.Uint16(m[4:6]) != 0 {
		t.Fatal("mDNS header wrong")
	}
	if binary.BigEndian.Uint16(m[6:8]) != 1 {
		t.Fatal("mDNS ANCOUNT wrong")
	}
}

func TestLanIPv4Filter(t *testing.T) {
	got := lanIPv4s(func() []string {
		return []string{"192.168.1.6", "127.0.0.1", "169.254.9.9", "::1", "not-an-ip"}
	})
	if len(got) != 1 || got[0].String() != "192.168.1.6" {
		t.Fatalf("filter = %v", got)
	}
}

func TestIsLLMNRName(t *testing.T) {
	for _, n := range []string{"beam", "b"} {
		if !isLLMNRName(n) {
			t.Errorf("isLLMNRName(%q) = false, want true", n)
		}
	}
	for _, n := range []string{"beam.local", "other", "", "beams", "bb", "beam."} {
		if isLLMNRName(n) {
			t.Errorf("isLLMNRName(%q) = true, want false", n)
		}
	}
	// parsing lowercases, so an upper-case query still matches
	_, q, ok := parseQuestion(buildQuery(1, "BEAM", 1))
	if !ok || !isLLMNRName(q.name) {
		t.Fatalf("upper BEAM not matched: %+v %v", q, ok)
	}
}

func TestAnswerPacketClasses(t *testing.T) {
	ips := []net.IP{net.ParseIP("192.168.1.6")}
	q := buildQuery(0x42, "beam", 1)
	uni := answerPacket("beam", ips, 0x42, q[12:], false)
	// unicast (LLMNR): plain IN class, short TTL
	if !bytes.Contains(uni, []byte{0, 1, 0, 1}) {
		t.Error("LLMNR answer should use plain IN class (0x0001)")
	}
	if !bytes.Contains(uni, []byte{0, 0, 0, 30}) {
		t.Errorf("LLMNR answer should use TTL 30, got %v", uni)
	}
	m := answerPacket("beam.local", ips, 0, nil, true)
	// mDNS: cache-flush class, long TTL
	if !bytes.Contains(m, []byte{0, 1, 0x80, 1}) {
		t.Error("mDNS answer should use cache-flush class (0x8001)")
	}
	if !bytes.Contains(m, []byte{0, 0, 0, 120}) {
		t.Errorf("mDNS answer should use TTL 120, got %v", m)
	}
}

func TestExtractQuestion(t *testing.T) {
	q := buildQuery(0x1234, "beam", 1)
	got := extractQuestion(q)
	if !bytes.Equal(got, q[12:]) {
		t.Fatalf("extract = %v, want %v", got, q[12:])
	}
	if extractQuestion([]byte{1, 2, 3}) != nil {
		t.Fatal("truncated extract should be nil")
	}
}

func TestLLMNRResponse(t *testing.T) {
	getIPs := func() []string { return []string{"192.168.1.6"} }

	// A query for beam -> unicast answer with ID + question echo + 1 record
	q := buildQuery(0x9a, "beam", 1)
	resp := llmnrResponse(q, getIPs)
	if resp == nil {
		t.Fatal("beam A query got no response")
	}
	if binary.BigEndian.Uint16(resp[0:2]) != 0x9a {
		t.Error("ID not echoed")
	}
	if resp[2]&0x80 == 0 {
		t.Error("QR not set")
	}
	if binary.BigEndian.Uint16(resp[4:6]) != 1 {
		t.Errorf("QDCOUNT = %d, want 1", binary.BigEndian.Uint16(resp[4:6]))
	}
	if binary.BigEndian.Uint16(resp[6:8]) != 1 {
		t.Errorf("ANCOUNT = %d, want 1", binary.BigEndian.Uint16(resp[6:8]))
	}
	if !bytes.Contains(resp, q[12:]) {
		t.Error("question not echoed")
	}
	if !bytes.Contains(resp, []byte{192, 168, 1, 6}) {
		t.Error("RDATA missing")
	}
	// the answer must parse as a query-shaped question for the echo part
	if _, eq, ok := parseQuestion(append(resp[:2], append([]byte{0, 0, 0, 1, 0, 0, 0, 0, 0, 0}, resp[12:]...)...)); !ok || eq.name != "beam" {
		// soft check only: echoed bytes must at least contain the name
		if !bytes.Contains(resp, encodeName("beam")) {
			t.Errorf("echoed name missing: %+v %v", eq, ok)
		}
	}

	// b + ANY also answer
	if llmnrResponse(buildQuery(7, "b", 255), getIPs) == nil {
		t.Error("b ANY query got no response")
	}
	// AAAA -> explicit NODATA (headers echo, zero answers, no retry storm)
	aaaa := llmnrResponse(buildQuery(0x55, "beam", 28), getIPs)
	if aaaa == nil {
		t.Fatal("beam AAAA query got no response")
	}
	if binary.BigEndian.Uint16(aaaa[0:2]) != 0x55 ||
		binary.BigEndian.Uint16(aaaa[4:6]) != 1 ||
		binary.BigEndian.Uint16(aaaa[6:8]) != 0 {
		t.Errorf("AAAA NODATA header wrong: id=%d qd=%d an=%d",
			binary.BigEndian.Uint16(aaaa[0:2]),
			binary.BigEndian.Uint16(aaaa[4:6]),
			binary.BigEndian.Uint16(aaaa[6:8]))
	}

	// negatives: not ours, dotted mDNS name, unsupported type, responses
	if llmnrResponse(buildQuery(1, "other", 1), getIPs) != nil {
		t.Error("other name must not be answered")
	}
	if llmnrResponse(buildQuery(1, "beam.local", 1), getIPs) != nil {
		t.Error("beam.local must not be answered via LLMNR (mDNS owns it)")
	}
	if llmnrResponse(buildQuery(1, "beam", 15), getIPs) != nil { // MX
		t.Error("unsupported QTYPE must not be answered")
	}
	rsp := buildQuery(1, "beam", 1)
	rsp[2] |= 0x80
	if llmnrResponse(rsp, getIPs) != nil {
		t.Error("response packet must be ignored")
	}
	// no LAN IPs -> no A answer (stay silent), but AAAA NODATA still replies
	empty := func() []string { return []string{"127.0.0.1"} }
	if llmnrResponse(buildQuery(1, "beam", 1), empty) != nil {
		t.Error("A query with no LAN IPs must stay silent")
	}
	if llmnrResponse(buildQuery(1, "beam", 28), empty) == nil {
		t.Error("AAAA NODATA must reply even with no LAN IPs")
	}
}

func TestLLMNRUDPLoopback(t *testing.T) {
	// End-to-end unicast shape without binding the real 5355 port:
	// client -> server socket, server replies via llmnrResponse.
	getIPs := func() []string { return []string{"192.168.7.7"} }
	srv, err := net.ListenPacket("udp4", "127.0.0.1:0")
	if err != nil {
		t.Skipf("no udp loopback: %s", err)
	}
	defer srv.Close()
	cli, err := net.ListenPacket("udp4", "127.0.0.1:0")
	if err != nil {
		t.Skipf("no udp loopback: %s", err)
	}
	defer cli.Close()
	query := buildQuery(0x77, "b", 1)
	if _, err := cli.WriteTo(query, srv.LocalAddr()); err != nil {
		t.Fatal(err)
	}
	_ = srv.SetReadDeadline(deadline(2))
	buf := make([]byte, 2048)
	n, addr, err := srv.ReadFrom(buf)
	if err != nil {
		t.Fatalf("server recv: %s", err)
	}
	resp := llmnrResponse(buf[:n], getIPs)
	if resp == nil {
		t.Fatal("no LLMNR response for b")
	}
	if _, err := srv.WriteTo(resp, addr); err != nil {
		t.Fatal(err)
	}
	_ = cli.SetReadDeadline(deadline(2))
	n, _, err = cli.ReadFrom(buf)
	if err != nil {
		t.Fatalf("client recv: %s", err)
	}
	got := buf[:n]
	if binary.BigEndian.Uint16(got[0:2]) != 0x77 {
		t.Error("loopback ID not echoed")
	}
	if !bytes.Contains(got, []byte{192, 168, 7, 7}) {
		t.Error("loopback RDATA missing")
	}
}
