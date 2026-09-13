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
func writeLog(ip, action, detail string) {
	ts := time.Now().Format("2006-01-02 15:04:05")
	line := ts + " | " + ip + " | " + action + " | " + detail
	logMu.Lock()
	logLines = append(logLines, line)
	if len(logLines) > memLogCap {
		logLines = append([]string{}, logLines[len(logLines)-memLogCap:]...)
	}
	logMu.Unlock()
	TouchActivity()
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
// StartIdleMonitor shuts the server down after IdleTimeout of silence.
func StartIdleMonitor() {
	if IdleTimeout <= 0 {
		return
	}
	go func() {
		t := time.NewTicker(idleTickInterval)
		defer t.Stop()
		for range t.C {
			if idleLeft() <= 0 {
				fmt.Printf("إغلاق تلقائي: مرت %s بدون أي نشاط.\n", IdleTimeout.String())
				go Shutdown()
				return
			}
		}
	}()
}
