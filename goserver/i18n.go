package beamcore

import (
	"fmt"
	"net/http"
	"strings"
)

// Server-side message catalog (Arabic + English).
// API responses carry the picked language in "msg"/"error".
// CLI banner and log lines stay Arabic (operator console).
var msgTable = map[string][2]string{
	// ---- admin surface ----
	"csrf":                {"طلب مرفوض (CSRF). حدث الصفحة وحاول تاني.", "Request rejected (CSRF). Refresh and retry."},
	"owner_only":          {"هذا الزر لصاحب الجهاز فقط — من جهاز التشغيل مباشرة.", "Owner only — from the host machine directly."},
	"owner_page":          {"هذه الصفحة لصاحب الجهاز فقط.", "This page is for the device owner only."},
	"logs_clear_fail":     {"تعذر مسح السجل. حاول تاني.", "Could not clear the log. Retry."},
	"logs_cleared":        {"اتمسح السجل بالكامل.", "Log cleared completely."},
	"bad_mode":            {"وضع غير صالح (lan أو hotspot).", "Invalid mode (lan or hotspot)."},
	"net_lan_ok":          {"تم التحويل لوضع LAN — اتصل بواي فاي ثم افتح %s", "Switched to LAN mode — join the Wi-Fi, then open %s"},
	"net_stop_ok":         {"%s السيرفر مستمر على LAN.", "%s Server continues on LAN."},
	"net_already":         {"الشبكة متوقفة بالفعل — السيرفر شغال على LAN.", "Network already stopped — server runs on LAN."},
	"port_not_number":     {"البورت لازم يكون رقماً — مثال: 2004", "Port must be a number — e.g. 2004"},
	"port_range":          {"البورت لازم بين 1 و 65535 — مثال: 2004", "Port must be 1–65535 — e.g. 2004"},
	"max_not_number":      {"الحد لازم يكون رقماً بالميجا — مثال: 20480", "Limit must be a number in MB — e.g. 20480"},
	"max_range":           {"الحد لازم بين 1 و 102400 ميجا.", "Limit must be 1–102400 MB."},
	"config_save_fail":    {"تعذر حفظ الإعدادات.", "Could not save settings."},
	"lang_bad":            {"اللغة غير صالحة (ar أو en).", "Invalid language (ar or en)."},
	"config_saved":        {"اتحفظت الإعدادات.", "Settings saved."},
	"config_restart_note": {" البورت الجديد يحتاج إعادة تشغيل السيرفر.", " The new port needs a server restart."},
	"server_stop_local":   {"إيقاف السيرفر من جهاز التشغيل فقط.", "Only the host machine can stop the server."},
	"server_stopped":      {"تم إيقاف السيرفر.", "Server stopped."},
	"delete_bad_name":     {"اسم غير صالح", "Invalid name"},
	"dir_bad":             {"مسار مجلد غير صالح", "Invalid folder path"},
	"dir_gone":            {"المجلد غير موجود", "Folder not found"},
	"zip_empty":           {"المجلد فارغ أو غير موجود", "Folder is empty or missing"},
	"zip_too_big":         {"المجلد أكبر من حد الضغط المباشر — نزّل ملفاته على دفعات", "Folder exceeds the direct-zip limit — download its files in batches"},
	"delete_missing":      {"غير موجود", "Not found"},
	"delete_busy":         {"الملف مستخدم حالياً (حد بينزّله). حدث القائمة وحاول تاني.", "File is in use right now (someone is downloading it). Refresh and retry."},
	// ---- hotspot core ----
	"hs_bad_port":     {"رقم البورت غير صالح. الحل: استخدم بورت بين 1 و 65535 (مثلاً 2004).", "Invalid port. Use 1–65535 (e.g. 2004)."},
	"hs_open_windows": {"ويندوز يشترط باسورد للهوتسبوت من النظام نفسه ولا يدعم الشبكة المفتوحة. الحل: اكتب باسورد 8+ حروف، أو استخدم جهاز لينكس للشبكة المفتوحة.", "Windows requires a hotspot password and blocks open networks. Use an 8+ char password, or a Linux machine for open mode."},
	"hs_short_pw":     {"كلمة مرور الشبكة قصيرة (%d/%d أحرف). الحل: اكتب باسورد من %d أحرف على الأقل.", "Network password too short (%d/%d chars). Use at least %d chars."},
	"hs_need_admin":   {"التشغيل يحتاج صلاحية المسؤول (Admin/sudo). الحل: شغّل البرنامج كمسؤول (ويندوز: كليك يمين Run as administrator / لينكس: sudo) ثم أعد التشغيل.", "Admin rights needed. Re-run as administrator (Windows: right-click Run as administrator / Linux: sudo)."},
	"hs_use_lan":      {"الحل البديل: استخدم وضع LAN وافتح %s", "Fallback: use LAN mode and open %s"},
	"hs_win_fail":     {"فشل تشغيل الهوتسبوت (WinRT: %s / netsh: %s). الحل: شغّل كمسؤول وتأكد من تفعيل الواي فاي ثم أعد المحاولة، ولو استمر استخدم وضع LAN.", "Hotspot failed (WinRT: %s / netsh: %s). Run as admin, enable Wi-Fi and retry, or use LAN mode."},
	"hs_no_nmcli":     {"أداة nmcli غير مثبتة. الحل: ثبّت NetworkManager ثم أعد المحاولة، أو استخدم وضع LAN: %s", "nmcli is missing. Install NetworkManager and retry, or use LAN mode: %s"},
	"hs_nmcli_fail":   {"فشل تشغيل الهوتسبوت (%s). الحل: شغّل بـ sudo وتأكد من عدم اتصال الواي فاي بشبكة أخرى، ثم أعد المحاولة أو استخدم وضع LAN.", "Hotspot failed (%s). Run with sudo, disconnect Wi-Fi from other networks and retry, or use LAN mode."},
	"hs_open_fail":    {"تعذر فتح الشبكة بدون باسورد (%s). الحل: كارت الواي فاي قد لا يدعم الوضع المفتوح — استخدم باسورد أو وضع LAN.", "Could not open the network (%s). The adapter may not support open mode — use a password or LAN mode."},
	"hs_started":      {"تم تشغيل شبكة '%s'. اتصل بها من الأجهزة ثم افتح %s%s", "Network '%s' is up. Join it from your devices, then open %s%s"}, "hs_warn_open": {" تنبيه: الشبكة مفتوحة بدون باسورد — أي جهاز قريب يقدر يدخل.", " Warning: open network — any nearby device can join."},
	"hs_verify_fail": {"الشبكة لم تستقر بعد التشغيل (غالباً تعارض مع واي فاي محفوظة أو الكارت). الحل: افصل الواي فاي عن الشبكات المحفوظة وأعد التشغيل، أو استخدم وضع LAN.", "The network did not stay up after starting (often a saved-Wi-Fi conflict or the adapter). Disconnect from saved networks and retry, or use LAN mode."},
	"hs_already":     {"الشبكة متوقفة بالفعل. لا حاجة لأي إجراء.", "Network already stopped. Nothing to do."},
	"hs_stop_fail":   {"فشل إيقاف الهوتسبوت (%s). الحل: نفّذ sudo nmcli connection down Hotspot يدوياً.", "Could not stop the hotspot (%s). Run: sudo nmcli connection down Hotspot."},
	"hs_stopped":     {"تم إيقاف الشبكة. الحل لو بقيت ظاهرة للأجهزة: أعد تشغيل الواي فاي.", "Network stopped. If devices still see it, restart Wi-Fi."},
	"hs_no_netsh":    {"تعذر فحص دعم الهوتسبوت: أداة netsh غير موجودة. الحل: استخدم جهازاً بنظام ويندوز حديثاً أو شغّل وضع LAN.", "Cannot check hotspot support: netsh missing. Use a modern Windows machine or LAN mode."},
	"hs_no_drivers":  {"تعذر فحص كارت الواي فاي (%s). الحل: تأكد من وجود كارت واي فاي وتعريفه، ثم أعد المحاولة، أو استخدم وضع LAN.", "Cannot inspect the Wi-Fi adapter (%s). Check the adapter and its driver, then retry or use LAN mode."},
	"hs_ap_ok":       {"كارت الواي فاي يدعم الهوتسبوت.", "The Wi-Fi adapter supports hotspot."},
	"hs_no_hosted":   {"كارت الواي فاي لا يدعم الهوتسبوت (Hosted network: No). الحل: دوس زر وضع LAN للعمل على واي فاي الموجودة.", "The adapter lacks hotspot support. Use LAN mode on the existing Wi-Fi."},
	"hs_unsure":      {"لم نستطع التأكد من دعم الهوتسبوت من مخرجات netsh. الحل: حدّث تعريف الواي فاي وحاول مجدداً، أو استخدم وضع LAN.", "Could not confirm hotspot support from netsh output. Update the driver and retry, or use LAN mode."},
	"hs_no_iw":       {"تعذر فحص دعم AP (%s). الحل: ثبّت حزمة iw أو استخدم وضع LAN.", "Cannot check AP support (%s). Install iw or use LAN mode."},
	"hs_ap_linux_ok": {"كارت الواي فاي يدعم وضع AP.", "The adapter supports AP mode."},
	"hs_no_ap":       {"كارت الواي فاي لا يدعم وضع AP. الحل: استخدم وضع LAN للعمل على شبكة الشركة الموجودة.", "The adapter lacks AP mode. Use LAN mode on the Wi-Fi."},
	"hs_assumed":     {"أداة iw غير مثبتة لكن nmcli موجود؛ يفترض دعم الهوتسبوت. الحل لو فشل التشغيل: ثبّت حزمة iw للفحص الدقيق أو استخدم وضع LAN.", "iw is missing but nmcli exists; hotspot support assumed. If it fails, install iw or use LAN mode."},
	"hs_no_tools":    {"لا توجد أدوات واي فاي (iw/nmcli غير مثبتة). الحل: ثبّت NetworkManager أو استخدم وضع LAN على الشبكة الحالية.", "No Wi-Fi tools (iw/nmcli missing). Install NetworkManager or use LAN mode."},
	"cmd_timeout":    {"انتهت المهلة أثناء تنفيذ: %s", "Timed out running: %s"},
	"cmd_not_found":  {"الأمر غير موجود: %s", "Command not found: %s"},
	"cmd_unexpected": {"خطأ غير متوقع: %s", "Unexpected error: %s"},
	"no_ps":          {"لا يوجد PowerShell", "PowerShell not found"},
	"ps_fail":        {"تعذر PowerShell", "PowerShell failed"},
	"wifi_off":       {"كارت الواي فاي مطفأ. الحل: شغّل الواي فاي من الإعدادات ثم أعد المحاولة.", "Wi-Fi adapter is off. Enable Wi-Fi in settings and retry."},
	"unknown_err":    {"خطأ غير معروف", "Unknown error"},
	"netsh_err":      {"خطأ netsh", "netsh error"},
	"nmcli_err":      {"خطأ nmcli", "nmcli error"},
	// ---- upload protocol ----
	"up_no_session":         {"لا توجد جلسة رفع بهذا الرقم", "No upload session with this id"},
	"up_bad_session":        {"جلسة غير صالحة. حدث الصفحة وحاول تاني.", "Invalid session. Refresh and retry."},
	"up_bad_name":           {"اسم ملف غير صالح", "Invalid file name"},
	"up_too_big":            {"حجم الملف خارج الحد المسموح", "File exceeds the allowed size"},
	"up_bad_piece_len":      {"حجم القطعة خارج الحد (256KB-16MB)", "Piece size out of range (256KB-16MB)"},
	"up_hash_count":         {"عدد البصمات لا يطابق حجم الملف", "Hash count does not match the file size"},
	"up_bad_hash":           {"بصمة قطعة غير صالح", "Invalid piece hash"},
	"up_session_gone":       {"انتهت الجلسة. ابدأ الرفع من جديد.", "Session expired. Start the upload again."},
	"up_chunk_no_len":       {"طول القطعة مفقود. أعد إرسال القطعة.", "Piece length missing. Resend the piece."},
	"up_chunk_over":         {"القطعة تتجاوز حجم الملف المعلن", "Piece exceeds the declared file size"},
	"up_write_fail":         {"فشل كتابة القطعة. حاول تاني.", "Could not write the piece. Retry."},
	"up_no_piece_session":   {"جلسة قطع غير موجودة. ابدأ الرفع من جديد.", "No piece session found. Start over."},
	"up_bad_index":          {"رقم قطعة غير صالح", "Invalid piece number"},
	"up_piece_len":          {"طول القطعة يجب أن يكون %d بايت", "Piece must be %d bytes"},
	"up_hash_required":      {"بصمة القطعة مطلوبة. أعد الإرسال مع hash.", "Piece hash required. Resend with hash."},
	"up_bad_pos":            {"موضع القطعة خارج حدود الملف", "Piece offset out of file bounds"},
	"up_piece_too_big":      {"القطعة أكبر من 64MB", "Piece larger than 64MB"},
	"up_conflict":           {"تعارض بصمات القطعة. ابدأ جلسة جديدة.", "Piece hash conflict. Start a new session."},
	"up_short_piece":        {"قطعة ناقصة. أعد إرسالها.", "Incomplete piece. Resend it."},
	"up_bad_pieces":         {"قطع تالفة أُسقطت — أعد إرسالها فقط", "Corrupt pieces dropped — resend only those"},
	"up_incomplete_n":       {"الملف ناقص (%d قطع متبقية). أكمل الرفع.", "File incomplete (%d pieces left). Finish the upload."},
	"up_incomplete_bytes":   {"الملف ناقص (%s من %s بايت). أكمل الرفع.", "File incomplete (%s of %s bytes). Finish the upload."},
	"up_verify_fail":        {"تعذر التحقق. حاول تاني.", "Could not verify. Retry."},
	"up_hash_mismatch":      {"بصمة الملف لا تطابق — أعد الرفع.", "File checksum mismatch — re-upload."},
	"up_fullhash_required":  {"التحقق النهائي مطلوب لجلسة التيربو (full_hash).", "Final verification is required for turbo sessions (full_hash)."},
	"zip_extract_fail":      {"تعذر فك الضغط — ملف الـ zip محفوظ كما هو.", "Could not extract — the zip file is kept as is."},
	"up_save_fail":          {"فشل حفظ %s. حاول تاني.", "Could not save %s. Retry."},
	"up_find_invalid":       {"اسم أو حجم غير صالح", "Invalid name or size"},
	"up_use_chunked":        {"استخدم الرفع المقسم من الصفحة للملفات الكبيرة", "Use the page chunked upload for large files"},
	"up_multipart_limit":    {"حجم الطلب خارج حد التوافق (128MB). استخدم الرفع المقسم من الصفحة.", "Request outside the compat limit (128MB). Use the page chunked upload."},
	"up_multipart_boundary": {"طلب غير صالح (لا توجد حدود multipart)", "Invalid request (no multipart boundary)"},
	"up_multipart_bad":      {"طلب غير صالح", "Invalid request"},
	"up_multipart_empty":    {"لم يصل أي ملف", "No file received"},
	"up_multipart_save":     {"فشل حفظ الملف. حاول تاني.", "Could not save the file. Retry."},
}

// tr picks the message in the requested language ("en" or anything else = ar).
func tr(lang, key string, args ...interface{}) string {
	pair, ok := msgTable[key]
	if !ok {
		return key
	}
	s := pair[0]
	if lang == "en" {
		s = pair[1]
	}
	if len(args) == 0 {
		return s
	}
	return fmt.Sprintf(s, args...)
}

// reqLang detects response language: explicit X-Lang header always wins,
// then the browser Accept-Language, default Arabic.
func reqLang(r *http.Request) string {
	xl := strings.ToLower(strings.TrimSpace(r.Header.Get("X-Lang")))
	if xl == "en" {
		return "en"
	}
	if xl == "ar" {
		return "ar"
	}
	al := strings.ToLower(r.Header.Get("Accept-Language"))
	for _, part := range strings.Split(al, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		tag := part
		if i := strings.Index(tag, ";"); i >= 0 {
			tag = strings.TrimSpace(tag[:i])
		}
		if tag == "en" || strings.HasPrefix(tag, "en-") {
			return "en"
		}
		if tag == "ar" || strings.HasPrefix(tag, "ar-") || tag == "*" {
			return "ar"
		}
	}
	return "ar"
}
