package beamcore

import (
	"fmt"
	"sync"
	"time"
)

var (
	logMu    sync.Mutex
	logLines []string
)

// writeLog appends one line to the in-memory ring (drops oldest past cap).
// Alloc-free steady state: shifts in place instead of allocating a fresh
// backing array on every line once full.
func writeLog(ip, action, detail string) {
	ts := time.Now().Format("2006-01-02 15:04:05")
	line := ts + " | " + ip + " | " + action + " | " + detail
	logMu.Lock()
	logLines = append(logLines, line)
	if len(logLines) > memLogCap {
		copy(logLines, logLines[len(logLines)-memLogCap:])
		logLines = logLines[:memLogCap]
	}
	logMu.Unlock()
	activityMu.Lock()
	lastActivity = time.Now()
	activityMu.Unlock()
}

// readLog returns the last n lines (oldest first).
func readLog(n int) []string {
	logMu.Lock()
	defer logMu.Unlock()
	if n <= 0 || n > len(logLines) {
		n = len(logLines)
	}
	out := make([]string, n)
	copy(out, logLines[len(logLines)-n:])
	return out
}

// clearLog wipes the in-memory log completely.
func clearLog() {
	logMu.Lock()
	logLines = []string{}
	logMu.Unlock()
}

var (
	activityMu   sync.Mutex
	lastActivity = time.Now()
)

func TouchActivity() {
	activityMu.Lock()
	lastActivity = time.Now()
	activityMu.Unlock()
}

// idleLeft reports how long until auto shutdown (<=0 means due now,
// MaxInt64 when the timeout is disabled).
func idleLeft() time.Duration {
	if IdleTimeout <= 0 {
		return time.Duration(1<<63 - 1)
	}
	activityMu.Lock()
	defer activityMu.Unlock()
	return IdleTimeout - time.Since(lastActivity)
}

// idleSecondsLeft is the JSON-friendly form for /api/status (-1 disabled).
func idleSecondsLeft() int64 {
	if IdleTimeout <= 0 {
		return -1
	}
	if d := idleLeft(); d > 0 {
		return int64(d / time.Second)
	}
	return 0
}

var (
	idleMu  sync.Mutex
	idleGen int64 // generation: bumped to supersede a running monitor
)

// StartIdleMonitor shuts the server down after IdleTimeout of silence.
// Restartable: every call supersedes any previous monitor, so the Android
// wrapper (in-process Stop/Start cycles) gets a live monitor per Start —
// the old fire-once goroutine died after the first Shutdown and never came
// back, leaving later generations unmonitored.
func StartIdleMonitor() {
	idleMu.Lock()
	idleGen++
	gen := idleGen
	idleMu.Unlock()
	if IdleTimeout <= 0 {
		return
	}
	go func() {
		t := time.NewTicker(idleTickInterval)
		defer t.Stop()
		for range t.C {
			idleMu.Lock()
			cur := idleGen
			idleMu.Unlock()
			if cur != gen {
				return // superseded by Stop/Restart (phone Start/Stop cycle)
			}
			if idleLeft() <= 0 {
				fmt.Printf("إغلاق تلقائي: مرت %s بدون أي نشاط.\n", IdleTimeout.String())
				go Shutdown()
				return
			}
		}
	}()
}

// StopIdleMonitor cancels any running monitor (phone Stop path — a stopped
// server must not auto-shutdown its own successor later).
func StopIdleMonitor() {
	idleMu.Lock()
	idleGen++
	idleMu.Unlock()
}

// RestartIdleMonitor stops any previous monitor and starts a fresh one.
// The Android wrapper calls this on every Start.
func RestartIdleMonitor() {
	StopIdleMonitor()
	StartIdleMonitor()
}
