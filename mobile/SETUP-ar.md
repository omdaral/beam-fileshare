# بناء تطبيق Beam للأندرويد (مرة واحدة للإعداد)

> الكود كامل وجاهز في `mobile/` — هذه الخطوات تثبت أدوات البناء فقط (~1.5GB تحميل لمرة واحدة)، ثم أمر واحد يبني الـ APK دائماً.

## 1) المتطلبات (تثبيت لمرة واحدة)

1. **Node.js 18+**: من https://nodejs.org (تحقق: `node -v`)
2. **JDK 21** (Capacitor 8 يشترط 21 — لا يكتفي بـ 17):
   - بدون صلاحيات (مجرّب هنا): نزّل Temurin 21 من https://adoptium.net → فك في `~/jdks` — سكربت البناء يكتشفه تلقائياً.
   - أو: `sudo apt install openjdk-21-jdk`.
   - (تحقق: `java -version` → 21+)
3. **Go 1.22+** (لمحرك السيرفر داخل التطبيق) + **NDK**:
   ```bash
   sdkmanager "ndk;26.3.11579264"
   ```
   أول بناء `./mobile/build-apk.sh` يثبت `gomobile` ويربط المحرك تلقائياً (يحتاج إنترنت أول مرة فقط).
3. **Android SDK**:
   ```bash
   mkdir -p ~/Android/Sdk/cmdline-tools
   # نزّل commandlinetools-linux من https://developer.android.com/studio#command-line-tools-only
   unzip commandlinetools-*.zip -d ~/Android/Sdk/cmdline-tools
   mv ~/Android/Sdk/cmdline-tools/cmdline-tools ~/Android/Sdk/cmdline-tools/latest
   export ANDROID_HOME=~/Android/Sdk
   export PATH=$ANDROID_HOME/cmdline-tools/latest/bin:$ANDROID_HOME/platform-tools:$PATH
   # ضع السطرين في ~/.bashrc ليثبتا دائماً
   sdkmanager --licenses  # وافق بـ y على الكل
   sdkmanager "platform-tools" "platforms;android-34" "build-tools;34.0.0"
   ```

## 2) البناء (كل مرة)

```bash
cd mobile
./build-apk.sh
# الناتج: dist/mobile/Beam-<ver>-android.apk
```

أول بناء يحمّل Gradle ومكونات أندرويد من الإنترنت (بطيء مرة واحدة فقط).

## 3) التثبيت على الموبايل

1. انسخ الـ APK للموبايل (USB/بلوتوث/Beam نفسه!) وافتحه.
2. وافق على "التثبيت من مصادر غير معروفة" لهذه المرة.
3. افتح Beam ← اكتب رابط السيرفر الظاهر بخط كبير على صفحة الكمبيوتر (أو 🔍 بحث تلقائي) ← دخول.
4. عند بدء رفع: إشعار دائم "Beam يرفع…" = الخدمة الأمامية تعمل — اقفل الشاشة وسيكمل.

## 4) قائمة التحقق على جهاز حقيقي

- [ ] رفع 200MB والشاشة مقفولة → يكتمل.
- [ ] تصغير التطبيق mid-upload → يكمل بدون إعادة.
- [ ] قتل التطبيق وإعادة فتحه → الاستكمال التلقائي يكمل الباقي.
- [ ] بعد انتهاء كل الرفوع: الإشعار يختفي (الخدمة توقفت ذاتياً).
- [ ] نفس الصفحة والمزايا (QR/بحث/تيربو) — لا فرق عن المتصفح.

## 5) لو الرفع توقف في الخلفية (احتياطي معروف)

بعض أجهزة (شاومي/هواوي/أوبو) تقتل التطبيقات عدوانياً: فعّل "التشغيل التلقائي" و"بدون قيود بطارية" لتطبيق Beam من إعدادات الموبايل. لو استمر التوقف، الحل الاحتياطي موثق في `goserver/web/index.html` (تعليق `BG-FALLBACK`: بلجن محلي صغير يستدعي `resumeTimers`).

## 6) ملاحظات

- الآيفون: هذه الحزمة أندرويد فقط — للآيفون استخدم "إضافة للشاشة الرئيسية" من سفاري (PWA-lite من السيرفر)، وبناء iOS يحتاج Mac.
- لا حاجة لحسابات مطورين أو متاجر — APK مباشر بتوقيع debug يكفي للتوزيع اليدوي.
