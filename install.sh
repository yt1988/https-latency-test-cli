#!/bin/sh
# httpslatency installer: picks the build for this OS and CPU, downloads it from 1ms.dog (which passes the GitHub
# release files on, for places where GitHub is slow or blocked) or else from GitHub itself, checks its SHA-256 against
# the release's checksums.txt, installs it as /usr/local/bin/httpslatency (or ~/.local/bin when not root) and runs it.
#
#   curl -fsSL https://1ms.dog/i | sh                     install, then run the test
#   curl -fsSL https://1ms.dog/i | sh -s -- -c ai,dev      install, then run with these options
#   curl -fsSL https://1ms.dog/i | sh -s -- --no-run       install only
#   curl -fsSL https://1ms.dog/i | sh -s -- run [ARGS]     download to a temp dir, run once, delete
#
# Environment: HTTPSLATENCY_VERSION (a tag such as v0.1.0; default latest), HTTPSLATENCY_DIR (install directory),
# HTTPSLATENCY_BASE (download base URL instead of 1ms.dog and GitHub), HTTPSLATENCY_NO_RUN=1 (install only).
set -eu

REPO="yt1988/https-latency-test-cli"
VERSION="${HTTPSLATENCY_VERSION:-latest}"
if [ "$VERSION" = latest ]; then
  BASES="${HTTPSLATENCY_BASE:-https://1ms.dog/dl/latest https://github.com/$REPO/releases/latest/download}"
else
  BASES="${HTTPSLATENCY_BASE:-https://1ms.dog/dl/$VERSION https://github.com/$REPO/releases/download/$VERSION}"
fi

say() { printf '%s\n' "$*" >&2; }
die() { say "httpslatency install: $*"; exit 1; }

os=$(uname -s | tr '[:upper:]' '[:lower:]')
case "$os" in linux|darwin|freebsd) ;; *) die "unsupported OS: $os (download a build from https://github.com/$REPO/releases)" ;; esac
arch=$(uname -m)
case "$arch" in
  x86_64|amd64) arch=amd64 ;;
  aarch64|arm64) arch=arm64 ;;
  armv7*|armv6*|armhf) arch=armv7 ;;
  i386|i686) arch=386 ;;
  riscv64) arch=riscv64 ;;
  mips|mipsel|mipsle)
    # OpenWrt and other MIPS routers: tell big from little endian by the ELF header of a system binary
    if [ "$(dd if=/bin/sh bs=1 skip=5 count=1 2>/dev/null | od -An -tx1 | tr -d ' ')" = 02 ]; then arch=mips; else arch=mipsle; fi ;;
  *) die "unsupported CPU: $arch" ;;
esac
file="httpslatency-$os-$arch"

if command -v curl >/dev/null 2>&1; then fetch() { curl -fsSL "$1" -o "$2"; }
elif command -v wget >/dev/null 2>&1; then fetch() { wget -qO "$2" "$1"; }
else die "needs curl or wget"; fi

tmp=$(mktemp -d 2>/dev/null || mktemp -d -t httpslatency)
trap 'rm -rf "$tmp"' EXIT INT TERM
say "Downloading $file ($VERSION)…"
got_it=
for base in $BASES; do
  if fetch "$base/$file" "$tmp/httpslatency" && fetch "$base/checksums.txt" "$tmp/checksums.txt"; then got_it=1; break; fi
  say "  not from $base, trying the next source"
done
[ -n "$got_it" ] || die "download failed from: $BASES"

want=$(grep " $file\$" "$tmp/checksums.txt" | cut -d' ' -f1)
[ -n "$want" ] || die "no checksum for $file"
if command -v sha256sum >/dev/null 2>&1; then got=$(sha256sum "$tmp/httpslatency" | cut -d' ' -f1)
elif command -v shasum >/dev/null 2>&1; then got=$(shasum -a 256 "$tmp/httpslatency" | cut -d' ' -f1)
elif command -v openssl >/dev/null 2>&1; then got=$(openssl dgst -sha256 "$tmp/httpslatency" | sed 's/.*= //')
else die "needs sha256sum, shasum or openssl to verify the download"; fi
[ "$got" = "$want" ] || die "checksum mismatch for $file (expected $want, got $got)"
chmod +x "$tmp/httpslatency"

# this script's input is the pipe from curl: with no options, the menu reads the keyboard from /dev/tty instead
has_tty() { { : </dev/tty; } 2>/dev/null; }

if [ "${1:-}" = run ]; then
  shift
  if [ $# -eq 0 ] && has_tty; then
    "$tmp/httpslatency" </dev/tty
  else
    "$tmp/httpslatency" "$@"
  fi
  exit $?
fi
no_run="${HTTPSLATENCY_NO_RUN:-}"
if [ "${1:-}" = --no-run ]; then no_run=1; shift; fi

if [ -n "${HTTPSLATENCY_DIR:-}" ]; then dir="$HTTPSLATENCY_DIR"
elif [ "$(id -u)" = 0 ] || [ -w /usr/local/bin ]; then dir=/usr/local/bin
else dir="$HOME/.local/bin"; fi
mkdir -p "$dir"
mv "$tmp/httpslatency" "$dir/httpslatency"
say "Installed $("$dir/httpslatency" --version) to $dir/httpslatency"
case ":$PATH:" in *":$dir:"*) ;; *) say "Add $dir to your PATH to run it by name." ;; esac

[ -n "$no_run" ] && exit 0
say ""
if [ $# -eq 0 ] && has_tty; then exec "$dir/httpslatency" </dev/tty; fi
exec "$dir/httpslatency" "$@"
