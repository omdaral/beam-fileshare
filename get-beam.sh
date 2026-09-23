#!/bin/bash
# Beam — one-click installer (auto-detects OS/arch, installs the right build, cleans up).
# Usage:
#   curl -sSL https://raw.githubusercontent.com/AhmedFaseh/beam-fileshare/main/get-beam.sh | bash -s -- --install
#   bash get-beam.sh [--install] [--version 1.7.1] [--dir ~/Beam] [--install-menu] [--no-extract] [--help]
# Detects: Linux/macOS/Windows(Git-Bash/MSYS/Cygwin/WSL) x amd64/arm64.
# Downloads from GitHub Releases (no build tools needed). stdlib only: sh + curl/wget + tar/unzip.
set -euo pipefail

REPO="AhmedFaseh/beam-fileshare"
DEFAULT_VER="1.7.1"
VER="$DEFAULT_VER"
DIR_ARG=""
DIR_GIVEN=0
EXTRACT=1
INSTALL=0
MENU=0
UNINSTALL=0

usage() {
  echo "Usage: get-beam.sh [--install] [--version X.Y.Z] [--dir PATH] [--install-menu] [--uninstall] [--no-extract] [--help]"
  echo "  --install: one-file-style install — download to a temp dir, then:"
  echo "             Linux: copy Beam into --dir (default ~/Beam, for the desktop icon)"
  echo "                    + install single-file command ~/.local/bin/beam"
  echo "                    + app-menu entry (no folder needed to run: just type 'beam')."
  echo "             macOS: folder install + single-file command (/usr/local/bin or ~/.local/bin)."
  echo "             The download is deleted afterwards (no leftovers)."
  echo "  --uninstall: remove the beam command, menu entry, icon and install dir."
  echo "  Without --install: just download + extract into --dir (default ./beam-download), archive kept."
}

while [ $# -gt 0 ]; do
  case "$1" in
    --version) VER="${2:-}"; shift 2 ;;
    --dir) DIR_ARG="${2:-}"; DIR_GIVEN=1; shift 2 ;;
    --install) INSTALL=1; shift ;;
    --install-menu) MENU=1; shift ;;
    --uninstall) UNINSTALL=1; shift ;;
    --no-extract) EXTRACT=0; shift ;;
    --help|-h) usage; exit 0 ;;
    *) echo "Unknown option: $1"; usage; exit 1 ;;
  esac
done

[ -z "$VER" ] && { echo "error: --version needs a value"; exit 1; }

# --- uninstall first (needs no download) ---
if [ "$UNINSTALL" = 1 ]; then
  if [ "$DIR_GIVEN" = 1 ]; then UDIR="$DIR_ARG"; else UDIR="${HOME:-$PWD}/Beam"; fi
  rm -f "$HOME/.local/bin/beam" /usr/local/bin/beam \
        "$HOME/.local/share/applications/beam.desktop" \
        "$HOME/.local/share/icons/hicolor/256x256/apps/beam.png" 2>/dev/null
  command -v update-desktop-database >/dev/null 2>&1 && update-desktop-database "$HOME/.local/share/applications" >/dev/null 2>&1 || true
  if [ -d "$UDIR" ]; then rm -rf "$UDIR" && echo "Removed dir: $UDIR"; fi
  echo "Beam uninstalled (command + menu entry + icon removed) ✅"
  exit 0
fi

# --- detect OS ---
OS_RAW="$(uname -s 2>/dev/null || echo unknown)"
ARCH_RAW="$(uname -m 2>/dev/null || echo unknown)"

case "$OS_RAW" in
  Linux*) OS="linux" ;;
  Darwin*) OS="macos" ;;
  MINGW*|MSYS*|CYGWIN*|Windows*) OS="windows" ;;
  *) echo "error: unsupported OS '$OS_RAW' (Linux/macOS/Windows only). See DOWNLOAD.md"; exit 1 ;;
esac

# WSL reports Linux — that is correct (use the linux bundle inside WSL).
if grep -qi microsoft /proc/version 2>/dev/null; then
  echo "note: WSL detected — downloading the Linux bundle (run it inside WSL)."
fi

# --- detect arch ---
case "$ARCH_RAW" in
  x86_64|amd64) ARCH="amd64" ;;
  aarch64|arm64|armv8*) ARCH="arm64" ;;
  *) echo "error: unsupported arch '$ARCH_RAW' (amd64/arm64 only). See DOWNLOAD.md"; exit 1 ;;
esac

# --- pick asset ---
case "$OS/$ARCH" in
  linux/amd64)   FILE="Beam-${VER}-linux-amd64.tar.gz" ;;
  linux/arm64)   FILE="Beam-${VER}-linux-arm64.tar.gz" ;;
  macos/amd64)   FILE="Beam-${VER}-macos-amd64.zip" ;;
  macos/arm64)   FILE="Beam-${VER}-macos-arm64.zip" ;;
  windows/amd64) FILE="Beam-${VER}-windows-amd64.zip" ;;
  windows/arm64) FILE="Beam-${VER}-windows-arm64.zip" ;;
  *) echo "error: no bundle for $OS/$ARCH"; exit 1 ;;
esac

URL="https://github.com/${REPO}/releases/download/v${VER}/${FILE}"
LATEST_URL="https://github.com/${REPO}/releases/latest/download/${FILE}"

# --- locations: install mode works in a temp dir (deleted afterwards) ---
TARGET=""
if [ "$INSTALL" = 1 ]; then
  EXTRACT=1 # --install always extracts (--no-extract is ignored with a note)
  TMPD="$(mktemp -d 2>/dev/null || echo "${TMPDIR:-/tmp}/beam-dl-$$")"
  mkdir -p "$TMPD"
  trap 'rm -rf "$TMPD"' EXIT
  OUTDIR="$TMPD/dl"
  if [ "$DIR_GIVEN" = 1 ]; then TARGET="$DIR_ARG"; else TARGET="${HOME:-$PWD}/Beam"; fi
else
  OUTDIR="${DIR_ARG:-./beam-download}"
fi

mkdir -p "$OUTDIR"
DEST="$OUTDIR/$FILE"

echo "Beam v${VER} — detected ${OS}/${ARCH}"
echo "Downloading: $URL"

download() {
  if command -v curl >/dev/null 2>&1; then
    curl -fSL --retry 3 -o "$DEST" "$1"
  elif command -v wget >/dev/null 2>&1; then
    wget -O "$DEST" "$1"
  else
    echo "error: need curl or wget installed."; exit 1
  fi
}

# Try pinned version first, fall back to latest/ (same filename) on 404.
if ! download "$URL"; then
  echo "Pinned v${VER} not found — trying latest/ ..."
  download "$LATEST_URL" || {
    echo "error: download failed. The repo may still be Private or the tag v${VER} was never pushed."
    echo "See DOWNLOAD.md section 5. Tried:"
    echo "  $URL"
    echo "  $LATEST_URL"
    exit 1
  }
fi

echo "Saved: $DEST ($(du -h "$DEST" | cut -f1))"

if [ "$EXTRACT" = 1 ]; then
  echo "Extracting..."
  case "$DEST" in
    *.tar.gz) tar xzf "$DEST" -C "$OUTDIR" ;;
    *.zip)
      if command -v unzip >/dev/null 2>&1; then
        unzip -o -q "$DEST" -d "$OUTDIR"
      elif command -v python3 >/dev/null 2>&1; then
        python3 -c "import zipfile,sys; zipfile.ZipFile(sys.argv[1]).extractall(sys.argv[2])" "$DEST" "$OUTDIR"
      else
        echo "error: need unzip or python3 to extract .zip"; exit 1
      fi
      ;;
  esac
  if [ "$INSTALL" = 1 ]; then
    # Bundle root: some bundles wrap files in a single folder, some do not.
    SRC="$OUTDIR"
    SUBS="$(find "$OUTDIR" -mindepth 1 -maxdepth 1 2>/dev/null)"
    if [ "$(printf '%s\n' "$SUBS" | wc -l)" = 1 ] && [ -d "$SUBS" ]; then SRC="$SUBS"; fi
    mkdir -p "$TARGET"
    rm -f "$DEST" # never copy the archive into the install (no leftovers)
    cp -a "$SRC/." "$TARGET/"
    rm -rf "$TMPD" # download + staging gone
    trap - EXIT
    echo ""
    echo "Installed Beam v${VER} → $TARGET (download cleaned up, no leftovers) ✅"
    case "$OS" in
      linux)
        if [ -x "$TARGET/install.sh" ]; then
          if [ "$MENU" = 1 ]; then ( cd "$TARGET" && ./install.sh --install-menu ); else ( cd "$TARGET" && ./install.sh ); fi
        fi
        # Single-file command: the Beam binary needs no sibling files at
        # runtime (version is embedded via ldflags), so one binary IS Beam.
        BIN_DIR="$HOME/.local/bin"
        mkdir -p "$BIN_DIR" 2>/dev/null || true
        if [ -x "$TARGET/Beam" ] && [ -d "$BIN_DIR" ]; then
          cp -f "$TARGET/Beam" "$BIN_DIR/beam" && chmod +x "$BIN_DIR/beam"
          echo "Single-file command installed: $BIN_DIR/beam ✅"
        fi
        # App-menu entry pointing at the single command (icon from the bundle).
        APPS_DIR="$HOME/.local/share/applications"
        ICON_DIR="$HOME/.local/share/icons/hicolor/256x256/apps"
        if [ -f "$TARGET/icon.png" ]; then mkdir -p "$ICON_DIR" 2>/dev/null && cp -f "$TARGET/icon.png" "$ICON_DIR/beam.png" 2>/dev/null || true; fi
        if [ -d "$APPS_DIR" ] && [ -x "$BIN_DIR/beam" ]; then
          cat > "$APPS_DIR/beam.desktop" <<EOF2
[Desktop Entry]
Version=1.0
Type=Application
Name=Beam
Name[en]=Beam
Comment=Share files between your devices over a private network - offline
Comment[ar]=مشاركة الملفات بين أجهزتك عبر شبكة خاصة - بدون إنترنت
Exec="$BIN_DIR/beam"
Icon=beam
Terminal=false
StartupNotify=true
Categories=Network;FileTransfer;
Keywords=share;files;beam;
StartupWMClass=Beam
EOF2
          chmod +x "$APPS_DIR/beam.desktop" 2>/dev/null || true
          if command -v gio >/dev/null 2>&1; then gio set "$APPS_DIR/beam.desktop" metadata::trusted true 2>/dev/null || true; fi
          if command -v update-desktop-database >/dev/null 2>&1; then update-desktop-database "$APPS_DIR" >/dev/null 2>&1 || true; fi
          echo "App-menu entry installed: $APPS_DIR/beam.desktop ✅"
        fi
        echo ""
        case ":$PATH:" in
          *":$BIN_DIR:"*) echo "Run from anywhere: beam" ;;
          *) echo "One-time PATH setup, then run 'beam' from anywhere:"; echo "  echo 'export PATH=\"\$HOME/.local/bin:\$PATH\"' >> ~/.bashrc && source ~/.bashrc" ;;
        esac
        echo "Or double-click the Beam icon, or: $TARGET/Beam.sh"
        ;;
      macos)
        if [ -x "$TARGET/Beam" ]; then
          if [ -d /usr/local/bin ] && [ -w /usr/local/bin ]; then
            cp -f "$TARGET/Beam" /usr/local/bin/beam && chmod +x /usr/local/bin/beam && echo "Single-file command: /usr/local/bin/beam ✅"
          else
            mkdir -p "$HOME/.local/bin" 2>/dev/null || true
            cp -f "$TARGET/Beam" "$HOME/.local/bin/beam" && chmod +x "$HOME/.local/bin/beam" && echo "Single-file command: $HOME/.local/bin/beam ✅ (add ~/.local/bin to PATH once)"
          fi
        fi
        echo "Next: open $TARGET — first time right-click Beam.command → Open" ;;
      windows) echo "Next: open $TARGET in Explorer — double-click Beam.bat" ;;
    esac
  else
    echo "Done. Files in $OUTDIR:"
    ls -la "$OUTDIR"
    echo ""
    case "$OS" in
      linux)   echo "Next: cd $OUTDIR && ./install.sh   # once, then double-click the Beam icon" ;;
      macos)   echo "Next: open $OUTDIR — first time right-click Beam.command → Open" ;;
      windows) echo "Next: open $OUTDIR in Explorer — double-click Beam.bat" ;;
    esac
  fi
else
  echo "Skipped extraction (--no-extract). File: $DEST"
fi

echo ""
echo "Verify integrity: compare sha256sum with SHA256SUMS on the Releases page."
