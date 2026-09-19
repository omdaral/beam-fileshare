package beamcore

import (
	"testing"
	"time"
)

// Restartable lifecycle (phone Stop/Start cycles in one process):
// Shutdown -> ResetShutdown -> Shutdown must close a fresh StopCh,
// idle monitor and name discovery must survive repeated Start/Stop.
func TestShutdownResetCycle(t *testing.T) {
	Shutdown()
	select {
	case <-StopCh:
	default:
		t.Fatal("StopCh not closed after Shutdown")
	}
	ResetShutdown()
	select {
	case <-StopCh:
		t.Fatal("StopCh still closed after ResetShutdown")
	default:
	}
	Shutdown()
	select {
	case <-StopCh:
	default:
		t.Fatal("StopCh not closed after second Shutdown")
	}
	ResetShutdown() // leave clean for other tests / desktop main
}

func TestIdleMonitorRestart(t *testing.T) {
	oldTimeout, oldTick := IdleTimeout, idleTickInterval
	IdleTimeout, idleTickInterval = 60*time.Millisecond, 10*time.Millisecond
	defer func() { IdleTimeout, idleTickInterval = oldTimeout, oldTick }()
	TouchActivity()
	StartIdleMonitor()
	RestartIdleMonitor() // must supersede, not pile up
	StopIdleMonitor()
	// A stopped monitor must not fire later and kill a successor.
	time.Sleep(50 * time.Millisecond)
	select {
	case <-StopCh:
		t.Fatal("stopped idle monitor fired Shutdown")
	default:
	}
	ResetShutdown()
}

func TestNameDiscoveryRestart(t *testing.T) {
	ips := func() []string { return []string{"192.168.1.2"} }
	StartNameDiscovery(ips)
	StartNameDiscovery(ips) // no-op, must not leak a second socket pair
	StopNameDiscovery()
	StopNameDiscovery()     // idempotent
	StartNameDiscovery(ips) // must rebind after Stop (or fail gracefully)
	StopNameDiscovery()
}

func TestPhoneLanIPPreferred(t *testing.T) {
	oldPhone, oldCache, oldAt := IsPhoneBuild, lanIPCache, lanIPAt
	oldPushed := PhoneLanIP()
	defer func() {
		IsPhoneBuild = oldPhone
		lanIPCache, lanIPAt = oldCache, oldAt
		SetPhoneLanIP(oldPushed)
	}()
	IsPhoneBuild = true
	lanIPCache = nil
	SetPhoneLanIP("192.168.43.1")
	ips := GetLANIPs()
	if len(ips) == 0 || ips[0] != "192.168.43.1" {
		t.Fatalf("pushed phone IP not first: %v", ips)
	}
	// Cached path must prefer it too (network switch takes effect at once).
	ips2 := GetLANIPs()
	if len(ips2) == 0 || ips2[0] != "192.168.43.1" {
		t.Fatalf("pushed phone IP not first on cache hit: %v", ips2)
	}
}
