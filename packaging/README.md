# Beam — التغليف والتوزيع (packaging/)

> الباينري static واحد لكل منصة (`./build-all.sh`)، وهنا يُغلَّف حسب كل نظام.
> Beam لا يكتب أي ملفات جانبية (لا config ولا logs): الإعدادات للجلسة فقط
> (+ نسخة في localStorage بالمتصفح)، والسجل في الذاكرة، والملفات في
> `~/Downloads/Beam` — أي حزمة قابلة لإعادة التثبيت دون مساس بملفات المستخدم.

## الخريطة

| الهدف | الملفات هنا | البناء | الحالة |
|---|---|---|---|
| Debian/Ubuntu `.deb` | `debian/build-deb.sh` + `common/` + `icons/` | `./packaging/debian/build-deb.sh amd64` | ✅ يُبنى ويُفحص (lintian) هنا |
| AppImage | `appimage/build-appimage.sh` | يحتاج `mksquashfs` (Debian: `sudo apt install squashfs-tools`) | ✅ x86_64 مُختبر تشغيلًا، aarch64 مبني فقط |
| Fedora `.rpm` | `fedora/build-rpm.sh` (spec + تاربول نظيف) | `./packaging/fedora/build-rpm.sh` (يحتاج `rpmbuild`: فيدورا `rpm-build` / ديبيان `rpm`) → `dist/rpm/` | ✅ يُبنى هنا ويُفحص (`rpm -qlp`) |
| Arch | `arch/PKGBUILD` + `arch/.SRCINFO` | على Arch: `updpkgsums && makepkg -si` | ملفات جاهزة، البناء على Arch |
| Flatpak/Flathub | `flatpak/com.beam.beam.{yaml,desktop,metainfo.xml,beam-wrapper.sh}` | يحتاج `flatpak-builder` (غير مثبت هنا) | manifest + metainfo متحقق منها |
| macOS | `Beam.command` (الجذر) + باينري darwin من `dist/` | zip جاهز | موثق في README (وضع LAN) |

## البناء الكلي
```bash
./build-all.sh                  # الباينريات الست + versions.json
./packaging/build-portables.sh  # حزم ZIP المحمولة الست في dist/portables/<ver>/
./packaging/build-packages.sh   # deb (amd64/arm64) + appimage (إن وُجد mksquashfs)
python3 packaging/manifest.py $(cat VERSION)  # فهرس dist/MANIFEST.json + SHA256SUMS
./publish.sh                    # الكل: 9 خطوات (قدرات + أيقونات + بناء + حزم + فهرس + لانشرات + تركيب + تحقق + تقرير)
```

## ملاحظات تقنية صادقة
1. **FUSE**: تشغيل AppImage يحتاج FUSE2 على جهاز المستخدم (أوبونتو 22.04+ بلا
   FUSE2 افتراضيًا) — البديل الموثق للمستخدم: `sudo apt install libfuse2`
   أو فك الحزمة يدويًا. البناء نفسه لا يحتاج FUSE (runtime + mksquashfs).
2. **الهوية قبل أي نشر عام (TODO)**: `beam-fileshare@localhost` مؤقت في
   (deb control/changelog) و`TODO` في (spec/PKGBUILD/metainfo/app-id) —
   ضع اسمك وبريدك وموقع المشروع وملف LICENSE قبل: Debian mentors / COPR /
   AUR / Flathub. ملاحظة: **app-id الخاص بـ Flathub لا يتغير بعد أول قبول**.
3. **Arch**: حدّث `source=` لرابط التاربول الحقيقي + `updpkgsums` ثم
   `makepkg --printsrcinfo > .SRCINFO` قبل الرفع على AUR.
4. **Flathub checklist**: ثبّت الهوية + LICENSE + حوّل مصدر الـ manifest إلى
   git tag + أضف screenshots بروابط + ابنِ بـ flatpak-builder + جرّب التشغيل
   (`flatpak-builder --run ... beam`) + اقرأ `flatpak-builder-lint`.
5. **Fedora/COPR**: ابنِ التاربول هنا (`make-tarball.sh`)، انسخه مع الـ spec
   لجهاز فيدورا، `rpmbuild -bb`، ثم ارفع SRPM إلى COPR.
6. المجلدات الفارغة تُفقد داخل zip المجلدات (limitation موثقة) — لا علاقة لها بالحزم.
7. `packaging/tools/` (appimagetool/runtime) أدوات بناء محلية غير مُcommitted
   منطقيًا — أضفها لـ `.gitignore` لو بدأت git (المجلد غير git حاليًا).

## مجلدات الإخراج (dist/)
- `dist/debian/*.deb`, `dist/appimage/*.AppImage` — من `build-packages.sh`.
- `dist/<os>/<arch>/<ver>/` + `versions.json` + `SHA256SUMS` — من `build-all.sh`.
