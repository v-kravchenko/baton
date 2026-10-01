#!/bin/sh
# Install baton on Linux, macOS or Termux.
#
#   curl -fsSL https://raw.githubusercontent.com/v-kravchenko/baton/main/install.sh | sh
#   curl -fsSL .../install.sh | sh -s -- --integrate claude,opencode
#
# Environment: BATON_INSTALL_DIR (default ~/.local/bin, $PREFIX/bin on Termux),
# BATON_VERSION (a tag such as v1.2.3; default latest).
set -eu

repo="v-kravchenko/baton"
integrate=""
while [ $# -gt 0 ]; do
	case "$1" in
	--integrate) integrate="$2"; shift 2 ;;
	--integrate=*) integrate="${1#*=}"; shift ;;
	-h|--help) sed -n '2,9p' "$0" 2>/dev/null || true; exit 0 ;;
	*) echo "install.sh: unknown argument $1" >&2; exit 2 ;;
	esac
done

fail() { echo "install.sh: $*" >&2; exit 1; }

case "$(uname -s)" in
Linux) os=linux ;;
Darwin) os=darwin ;;
*) fail "unsupported OS $(uname -s); on Windows use install.ps1" ;;
esac
case "$(uname -m)" in
x86_64|amd64) arch=amd64 ;;
aarch64|arm64) arch=arm64 ;;
armv7*|armv8l) arch=armv7 ;;
*) fail "unsupported architecture $(uname -m)" ;;
esac
asset="baton_${os}_${arch}"

termux=""
case "${PREFIX:-}" in *com.termux*) termux=1 ;; esac
if [ -n "${BATON_INSTALL_DIR:-}" ]; then
	dir="$BATON_INSTALL_DIR"
elif [ -n "$termux" ]; then
	dir="$PREFIX/bin"
else
	dir="$HOME/.local/bin"
fi

if [ -n "${BATON_VERSION:-}" ]; then
	base="https://github.com/$repo/releases/download/$BATON_VERSION"
else
	base="https://github.com/$repo/releases/latest/download"
fi

if command -v curl >/dev/null 2>&1; then
	fetch() { curl -fsSL -o "$2" "$1"; }
elif command -v wget >/dev/null 2>&1; then
	fetch() { wget -qO "$2" "$1"; }
else
	fail "need curl or wget"
fi

if command -v sha256sum >/dev/null 2>&1; then
	sha() { sha256sum "$1" | cut -d' ' -f1; }
elif command -v shasum >/dev/null 2>&1; then
	sha() { shasum -a 256 "$1" | cut -d' ' -f1; }
else
	fail "need sha256sum or shasum"
fi

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT INT TERM
echo "downloading $asset"
fetch "$base/$asset" "$tmp/baton"
fetch "$base/checksums.txt" "$tmp/checksums.txt"
want="$(awk -v n="$asset" '$2 == n || $2 == "*" n { print $1 }' "$tmp/checksums.txt")"
[ -n "$want" ] || fail "checksums.txt has no entry for $asset"
got="$(sha "$tmp/baton")"
[ "$got" = "$want" ] || fail "SHA256 mismatch for $asset: got $got, want $want"

mkdir -p "$dir"
chmod 755 "$tmp/baton"
mv -f "$tmp/baton" "$dir/baton"
echo "installed $dir/baton ($("$dir/baton" version))"

"$dir/baton" config init

case ":$PATH:" in
*":$dir:"*) ;;
*) echo "note: $dir is not on PATH; add it to your shell profile" ;;
esac

if [ -n "$integrate" ]; then
	for a in $(echo "$integrate" | tr ',' ' '); do
		"$dir/baton" integrate "$a"
	done
fi
