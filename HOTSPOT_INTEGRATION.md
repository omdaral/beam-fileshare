# تكامل الهوتسبوت مع السيرفر (Go)

> موديول الشبكة الآن جزء من الباينري: `goserver/net.go` (stdlib فقط).

## 1. مثال الاستدعاء من السيرفر

```go
// تشغيل الشبكة (SSID افتراضي Beam، باسورد 8+ أحرف)
ok, msg, info := hotspotStart("Beam", "password123", 2004, false)
fmt.Println(msg)
if ok {
    fmt.Println("الرابط:", info["url"]) // مثال: http://192.168.137.1:2004
} else {
    // الهوتسبوت مستحيل؟ استخدم وضع LAN البديل
    fb := lanFallbackInfo(2004)
    fmt.Println(fb["message_ar"], fb["url"])
}

// الحالة والإيقاف
fmt.Println(hotspotStatus()) // {running, ssid, ip, port, clients}
ok, msg = hotspotStop()
```

## 2. أعلام سطر الأوامر (في `goserver/main.go`)

```
--hotspot                تشغيل هوتسبوت قبل السيرفر
--ssid Beam     اسم الشبكة
--password password123   باسورد الهوتسبوت (8+ أحرف)
--open                   شبكة مفتوحة بدون باسورد (لينكس فقط)
--lan-mode               تخطي الهوتسبوت والعمل على الشبكة الحالية
--port 2004              البورت الثابت (الافتراضي 2004)
--lang ar                اللغة الافتراضية للزوار الجدد (ar/en)
--no-browser             عدم فتح المتصفح تلقائياً عند التشغيل
--idle-timeout 5h        إغلاق تلقائي بعد الخمول (0 للتعطيل)
```

- `--hotspot --ssid Beam --password password123` → يستدعي `hotspotStart(...)` ثم يشغّل السيرفر.
- `--lan-mode` (أو فشل `start`) → يستدعي `lanFallbackInfo(port)` ويطبع `url` للعملاء.
- عند الإيقاف (Ctrl+C) → يستدعي `hotspotStop()`.
- من المتصفح (جهاز التشغيل): `POST /api/net/start` بنفس الخيارات — ولو نقصت الصلاحية
  حاول السيرفر عبر `pkexec` تلقائياً (بوب-أب النظام على جهازك فقط)، وإلا fallback لـ LAN.
