package beamcore

import (
	"testing"
	"time"
)

func TestLookupHostBoundedLocalhost(t *testing.T) {
	start := time.Now()
	ips := lookupHostBounded("localhost")
	if time.Since(start) > 10*time.Second {
		t.Fatalf("localhost lookup took too long: %v", time.Since(start))
	}
	found := false
	for _, ip := range ips {
		if ip == "127.0.0.1" || ip == "::1" {
			found = true
		}
	}
	if !found {
		t.Fatalf("localhost should resolve to loopback, got %v", ips)
	}
}

func TestLookupHostBoundedEmpty(t *testing.T) {
	if ips := lookupHostBounded(""); ips != nil {
		t.Fatalf("empty host should return nil, got %v", ips)
	}
}

func TestLookupHostBoundedInvalidFast(t *testing.T) {
	// Must return within a small multiple of the timeout even when
	// DNS hangs — this is what used to stall /api/status on phones.
	start := time.Now()
	_ = lookupHostBounded("nonexistent-host-invalid-xyz Beam")
	if d := time.Since(start); d > 3*dnsLookupTimeout {
		t.Fatalf("bounded lookup took %v, want < %v", d, 3*dnsLookupTimeout)
	}
}
