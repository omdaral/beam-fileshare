// Beam desktop CLI: thin wrapper over the beamcore server engine
// (the same engine also powers the Android wrapper via gomobile).
package main

import (
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	core "fileshare"
)

func main() {
	core.ApplyEnvOverrides()
	core.BaseDir = core.ExeDir()
	core.Cfg = core.LoadConfig()
	core.AppVersion = core.LoadVersion(core.BaseDir)

	portFlag := flag.Int("port", core.Cfg.Port, "server port (fixed default 2004: http://<ip>:2004)")
	hotspotFlag := flag.Bool("hotspot", false, "start hotspot before server")
	ssidFlag := flag.String("ssid", core.Cfg.SSID, "hotspot SSID")
	passwordFlag := flag.String("password", core.Cfg.HotspotPassword, "hotspot password")
	openFlag := flag.Bool("open", false, "open hotspot without password (linux only)")
	lanFlag := flag.Bool("lan-mode", false, "use current network without hotspot")
	langFlag := flag.String("lang", core.Cfg.DefaultLang, "default UI language for new visitors (ar or en)")
	noBrowserFlag := flag.Bool("no-browser", false, "do not auto-open the web UI in the browser")
	tlsFlag := flag.Bool("tls", false, "serve HTTPS with a self-signed LAN cert (https://<ip>:port)")
	noTLSFlag := flag.Bool("no-tls", false, "deprecated: HTTP is now the default (kept for compat)")
	tlsRegenFlag := flag.Bool("tls-regen", false, "regenerate the self-signed TLS cert on boot")
	// Default honors BEAM_IDLE_TIMEOUT (ApplyEnvOverrides ran before flags).
	idleFlag := flag.Duration("idle-timeout", core.IdleTimeout, "auto shutdown after this long without activity (0 disables, e.g. 30m, 5h)")
	flag.Parse()

	port := *portFlag
	core.IdleTimeout = *idleFlag
	core.TouchActivity() // boot counts as activity
	if core.ValidatePort(port) < 0 {
		fmt.Printf("رقم البورت غير صالح (%d). الحل: استخدم بورت بين 1 و 65535 (مثلاً 2004)، مثال: Beam --port 2004\n", port)
		os.Exit(2)
	}
	core.ServerPort = port

	// Language default for new visitors (session only; the owner's
	// browser keeps its own copy in localStorage).
	core.CfgMu.Lock()
	core.Cfg.Port = port
	core.Cfg.DefaultLang = core.NormalizeLang(*langFlag)
	core.CfgMu.Unlock()

	// The one and only writable location: ~/Downloads/Beam-Temp.
	// (The old ~/Downloads/Beam folder + Shared/ migration are gone.)
	core.TempDir = core.TempDefaultDir()
	core.SharedDir = core.TempDir // deprecated alias stays pointed at temp
	core.Cfg.TempDir = core.TempDir
	_ = os.MkdirAll(core.TempDir, 0755)

	// TLS: plain HTTP by default on desktop (BEAM_TLS=1 or --tls for HTTPS).
	tlsOn := *tlsFlag && !*noTLSFlag
	switch v := tlsEnv(); v {
	case 1:
		tlsOn = true
	case -1:
		tlsOn = false
	}
	core.TLSEnabled = tlsOn
	if core.TLSEnabled && *tlsRegenFlag {
		core.DropTLSCert()
	}
	if core.TLSEnabled {
		if _, _, err := core.EnsureTLSCert(); err != nil {
			fmt.Printf("تعذر تجهيز شهادة HTTPS (%s) — نكمل بـ HTTP.\n", err)
			core.TLSEnabled = false
		}
	}

	// Restore pre-restart temp files FIRST so they count as live and are
	// never swept: loose Beam-Temp files come back with original names,
	// legacy hidden .shares orphans are migrated to real managed files.
	loose, adopted := core.RestoreTempShares()
	if loose+adopted > 0 {
		fmt.Printf("  استعادة مشاركات سابقة: %d ملفات + %d ملفات قديمة بأسمائها\n", loose, adopted)
	}
	core.SweepUploads()
	core.SweepTempOrphans()

	core.NSMu.Lock()
	core.Net.SSID = *ssidFlag
	if core.Net.SSID == "" {
		core.Net.SSID = "Beam"
	}
	core.Net.LanMode = true
	core.Net.Security = "wpa"
	core.Net.WifiPassword = ""
	core.NSMu.Unlock()

	useHotspot := (*hotspotFlag || core.Cfg.NetMode == "hotspot" || core.Cfg.Mode == "hotspot") && !*lanFlag
	openNet := *openFlag || core.Cfg.WifiOpen
	if useHotspot {
		pw := *passwordFlag
		if pw == "" {
			pw = core.Cfg.HotspotPassword
		}
		if !openNet && pw == "" {
			fmt.Println("لازم باسورد 8+ حروف مع الهوتسبوت (أو --open لشبكة مفتوحة على لينكس). " +
				"الحل: أضف --password مثال: --password 12345678")
		} else {
			ok, msg, info := core.HotspotStart(*ssidFlag, pw, port, openNet, "ar")
			fmt.Println(msg)
			if ok {
				// Confirm the network really stayed up (a false "ok"
				// used to flip the UI and silently revert on next poll).
				if st := core.HotspotStatus(); !st["running"].(bool) {
					_, _ = core.HotspotStop("ar")
					fmt.Println("الشبكة لم تستقر — نكمل بوضع LAN.")
				} else {
					core.NSMu.Lock()
					core.Net.LanMode = false
					core.Net.HotspotRunning = true
					if s, ok := info["ssid"].(string); ok {
						core.Net.SSID = s
					} else {
						core.Net.SSID = *ssidFlag
					}
					if s, ok := info["security"].(string); ok {
						core.Net.Security = s
					} else if openNet {
						core.Net.Security = "open"
					}
					if !openNet {
						core.Net.WifiPassword = pw
					}
					core.NSMu.Unlock()
				}
			} else {
				fb := core.LanFallbackInfo(port)
				fmt.Println(fb["message_ar"])
			}
		}
	}

	localURL := core.TLSScheme() + "://127.0.0.1:" + strconv.Itoa(port)

	// Port taken before we bound: don't start a second copy and don't
	// print a scary error — just open the running instance's UI.
	if core.IsPortOpen(port) {
		if core.JoinRunningInstance(port, *noBrowserFlag, localURL) {
			os.Exit(0)
		}
		fmt.Printf("البورت %d مشغول ببرنامج آخر (ليس Beam). الحل: أوقف البرنامج الآخر أو شغل على بورت آخر، مثال: Beam --port %d\n", port, port+1)
		os.Exit(2)
	}

	go func() {
		if err := core.Run(port); err != nil && err != http.ErrServerClosed {
			if core.JoinRunningInstance(port, *noBrowserFlag, localURL) {
				os.Exit(0)
			}
			fmt.Printf("تعذر فتح البورت %d (%s). الحل: جرب بورت آخر.\n", port, err)
			os.Exit(2)
		}
	}()

	// Wait briefly until our listener is up.
	up := false
	for i := 0; i < 50; i++ {
		time.Sleep(100 * time.Millisecond)
		if core.IsPortOpen(port) {
			up = true
			break
		}
	}
	if !up {
		if core.JoinRunningInstance(port, *noBrowserFlag, localURL) {
			os.Exit(0)
		}
		fmt.Printf("البورت %d مشغول ببرنامج آخر. الحل: أوقف النسخة القديمة أو شغل على بورت آخر، مثال: Beam --port %d\n", port, port+1)
		os.Exit(2)
	}

	fmt.Println("==================================================")
	fmt.Printf("  Beam v%s شغال\n", core.AppVersion)
	fmt.Printf("  فولدر المشاركة: %s\n", core.SharedDir)
	fmt.Println("  الأمان: باسورد شبكة الواي فاي فقط — أي جهاز على الشبكة يدخل مباشرة")
	if core.TLSEnabled {
		fmt.Println("  التشفير: HTTPS بشهادة ذاتية للشبكة المحلية (اقبل التحذير أول مرة)")
		if core.TLSFingerprint != "" {
			fmt.Printf("  بصمة الشهادة: %s\n", core.TLSFingerprint)
		}
	}
	fmt.Printf("  افتح من نفس الجهاز: %s\n", localURL)
	for _, ip := range core.GetLANIPs() {
		fmt.Printf("  من الموبايلات/الأجهزة: %s\n", core.BaseURL(ip, port))
	}
	if core.IdleTimeout > 0 {
		fmt.Printf("  إغلاق تلقائي بعد %s بدون أي نشاط (زر الإيقاف الدائري في أعلى الصفحة للإيقاف اليدوي)\n", core.IdleTimeout.String())
	}
	fmt.Println("  للإيقاف: زر «إيقاف السيرفر» الدائري في أعلى الصفحة، أو Ctrl+C هنا")
	fmt.Println("==================================================")

	// Pretty LAN names (best effort): http://beam.local:2004 everywhere,
	// http://beam:2004 on Windows. Plain IPs always work regardless.
	core.StartNameDiscovery(core.GetLANIPs)

	// The server opens its own web UI: double-clicking the Beam binary
	// is enough on every OS (no launcher scripts needed for this).
	if !*noBrowserFlag {
		core.OpenBrowser(localURL)
	}
	core.StartIdleMonitor()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	select {
	case <-sigCh:
	case <-core.StopCh: // ordered via /api/server/stop: exit the process too
	}
	fmt.Println("\nتم الإيقاف.")
	core.NSMu.RLock()
	hsRunning := core.Net.HotspotRunning
	core.NSMu.RUnlock()
	if hsRunning {
		if ok, msg := core.HotspotStop("ar"); ok {
			fmt.Println(msg)
		}
	}
}

// tlsEnv reads BEAM_TLS: 1/true/yes -> force on, 0/false/no -> force off,
// anything else (unset) -> 0 = no override.
func tlsEnv() int {
	v := os.Getenv("BEAM_TLS")
	if v == "" {
		return 0
	}
	switch v {
	case "1", "true", "TRUE", "yes", "YES", "on", "ON":
		return 1
	case "0", "false", "FALSE", "no", "NO", "off", "OFF":
		return -1
	default:
		return 0
	}
}
