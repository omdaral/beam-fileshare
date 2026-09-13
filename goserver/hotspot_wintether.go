package beamcore

import (
	"regexp"
	"strings"
	"time"
)

const psTetherPreamble = `$ErrorActionPreference='Stop';` +
	`Add-Type -AssemblyName System.Runtime.WindowsRuntime;` +
	`$_m=[System.WindowsRuntimeSystemExtensions].GetMethods()|` +
	`Where-Object{$_.Name -eq 'AsTask' -and $_.GetParameters().Count -eq 1};` +
	`function Await($op){` +
	`$t=$_m.MakeGenericMethod($op.GetType().GetGenericArguments()[0]).Invoke($null,@($op));` +
	`$t.Wait(-1)|Out-Null;return $t.Result;}` +
	`$_cp=[Windows.Networking.Connectivity.NetworkInformation,` +
	`Windows.Networking.Connectivity,ContentType=WindowsRuntime]::` +
	`GetInternetConnectionProfile();` +
	`$_tm=[Windows.Networking.NetworkOperators.NetworkOperatorTetheringManager,` +
	`Windows.Networking.NetworkOperators,ContentType=WindowsRuntime]::` +
	`CreateFromConnectionProfile($_cp);`

func psQuote(s string) string {
	return strings.ReplaceAll(s, "'", "''")
}
func winPS(script string, timeout time.Duration, lang string) (int, string, string) {
	exe := ""
	if hasCommand("powershell") {
		exe = "powershell"
	} else if hasCommand("pwsh") {
		exe = "pwsh"
	} else {
		return 127, "", tr(lang, "no_ps")
	}
	return runCmd([]string{exe, "-NoProfile", "-ExecutionPolicy", "Bypass",
		"-Command", script}, timeout, lang)
}
func winTetherStart(ssid, password, lang string) (bool, string) {
	script := psTetherPreamble +
		`$_c=New-Object Windows.Networking.NetworkOperators.` +
		`NetworkOperatorTetheringAccessPointConfiguration;` +
		`$_c.Ssid='` + psQuote(ssid) + `';$_c.Passphrase='` + psQuote(password) + `';` +
		`'CFG:'+(Await ($_tm.ConfigureAccessPointAsync($_c))).Status;` +
		`'RUN:'+(Await ($_tm.StartTetheringAsync())).Status;`
	code, out, errStr := winPS(script, 30*time.Second, lang)
	if code != 0 {
		detail := strings.TrimSpace(errStr)
		if detail == "" {
			detail = strings.TrimSpace(out)
		}
		if detail == "" {
			detail = tr(lang, "ps_fail")
		}
		if len(detail) > 200 {
			detail = detail[:200]
		}
		return false, detail
	}
	if strings.Contains(out, "RUN:Success") {
		return true, ""
	}
	if strings.Contains(out, "WiFiDeviceOff") {
		return false, tr(lang, "wifi_off")
	}
	m := regexp.MustCompile(`(?:CFG|RUN):(\w+)`).FindStringSubmatch(out)
	status := ""
	if m != nil {
		status = m[1]
	} else {
		status = strings.TrimSpace(out)
		if len(status) > 120 {
			status = status[:120]
		}
		if status == "" {
			status = tr(lang, "unknown_err")
		}
	}
	return false, status
}
func winTetherStop(lang string) (bool, string) {
	script := psTetherPreamble + `'STOP:'+(Await ($_tm.StopTetheringAsync())).Status;`
	code, out, errStr := winPS(script, 30*time.Second, lang)
	if code != 0 {
		detail := strings.TrimSpace(errStr)
		if detail == "" {
			detail = strings.TrimSpace(out)
		}
		if len(detail) > 200 {
			detail = detail[:200]
		}
		return false, detail
	}
	if strings.Contains(out, "STOP:Success") || strings.Contains(out, "STOP:OperationInProgress") {
		return true, ""
	}
	m := regexp.MustCompile(`STOP:(\w+)`).FindStringSubmatch(out)
	if m != nil {
		return false, m[1]
	}
	detail := strings.TrimSpace(out)
	if len(detail) > 120 {
		detail = detail[:120]
	}
	return false, detail
}
func winTetherState() string {
	script := psTetherPreamble + `'STATE:'+$_tm.TetheringOperationalState;`
	code, out, _ := winPS(script, 15*time.Second, "ar")
	if code != 0 {
		return ""
	}
	m := regexp.MustCompile(`STATE:(\w+)`).FindStringSubmatch(out)
	if m == nil {
		return ""
	}
	return m[1]
}

var netshStoppedValues = []string{
	"not started", "non démarré", "nicht gestartet", "no iniciado",
	"não iniciado", "non avviato", "لم يبدأ", "متوقف", "غير مشغل",
	"غير مشغّل", "موقوف",
}

func netshShowsStopped(output string) bool {
	for _, line := range strings.Split(output, "\n") {
		if !strings.Contains(line, ":") {
			continue
		}
		idx := strings.Index(line, ":")
		key, val := strings.TrimSpace(line[:idx]), strings.TrimSpace(line[idx+1:])
		if !strings.Contains(strings.ToLower(key), "status") &&
			!strings.Contains(key, "الحالة") &&
			!strings.Contains(strings.ToLower(key), "état") {
			continue
		}
		v := strings.ToLower(val)
		for _, s := range netshStoppedValues {
			if strings.Contains(v, s) {
				return true
			}
		}
	}
	return false
}
