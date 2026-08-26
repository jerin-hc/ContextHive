#!/usr/bin/env bash
set -euo pipefail

# ---------------------------------------------------------------------------
# install.sh — install Go (if needed), build the MCP server, and drop the
# binary into ./bin (created if it doesn't exist).
#
# Usage:
#   ./install.sh                        # builds <script dir>/bin/<dir name>
#   BIN_DIR=/opt/ctxhive ./install.sh   # override output directory
#   GO_VERSION=1.28.0 ./install.sh      # override the Go version to install
#
# Install location: /usr/local/go (via sudo if needed), falling back to
# ~/.local/go when /usr/local isn't writable. An existing Go >= the version
# in go.mod is reused as-is.
# ---------------------------------------------------------------------------

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
APP_NAME="$(basename "$SCRIPT_DIR")"

GO_VERSION="${GO_VERSION:-1.27.0}"          # matches `go 1.27.0` in go.mod
BIN_DIR="${BIN_DIR:-$SCRIPT_DIR/bin}"
GO_TARBALL_PREFIX="${GO_TARBALL_PREFIX:-/usr/local}" # go lives at $prefix/go
LOCAL_GO_PREFIX="${LOCAL_GO_PREFIX:-$HOME/.local}"   # no-sudo fallback

log() { printf '\033[1;34m[install]\033[0m %s\n' "$*"; }
die() { printf '\033[1;31m[install] error:\033[0m %s\n' "$*" >&2; exit 1; }

# --- detect OS/arch ---------------------------------------------------------
case "$(uname -s)" in
  Linux)  GO_OS="linux" ;;
  Darwin) GO_OS="darwin" ;;
  *) die "unsupported OS: $(uname -s)" ;;
esac
case "$(uname -m)" in
  x86_64|amd64) GO_ARCH="amd64" ;;
  aarch64|arm64) GO_ARCH="arm64" ;;
  *) die "unsupported architecture: $(uname -m)" ;;
esac

# ver_ge installed required — exit 0 when installed >= required
ver_ge() {
  awk -v a="$1" -v b="$2" 'BEGIN {
    split(a, A, "."); split(b, B, ".")
    for (i = 1; i <= 3; i++) {
      if (A[i]+0 > B[i]+0) exit 0
      if (A[i]+0 < B[i]+0) exit 1
    }
    exit 0
  }'
}

go_satisfies() { # $1 = path to a go binary
  local candidate="$1" version
  [ -x "$candidate" ] || return 1
  version="$("$candidate" version 2>/dev/null | awk '{print $3}')" || return 1
  version="${version#go}"
  [[ "$version" =~ ^[0-9]+\.[0-9]+ ]] || return 1
  ver_ge "$version" "$GO_VERSION"
}

# --- locate an existing Go, or install one ----------------------------------
GO=""
if go_satisfies "$(command -v go 2>/dev/null || true)"; then
  GO="$(command -v go)"
  log "using existing Go: $("$GO" version | awk '{print $3}') ($GO)"
elif go_satisfies "/usr/local/go/bin/go"; then
  GO="/usr/local/go/bin/go"
  log "using existing Go: $("$GO" version | awk '{print $3}') ($GO)"
else
  TARBALL="go${GO_VERSION}.${GO_OS}-${GO_ARCH}.tar.gz"
  URL="https://go.dev/dl/${TARBALL}"
  TMP_DIR="$(mktemp -d)"
  trap 'rm -rf "$TMP_DIR"' EXIT

  log "downloading ${TARBALL} ..."
  if command -v curl >/dev/null 2>&1; then
    curl -fL --retry 3 -o "$TMP_DIR/$TARBALL" "$URL"
  elif command -v wget >/dev/null 2>&1; then
    wget -q -O "$TMP_DIR/$TARBALL" "$URL"
  else
    die "neither curl nor wget is available"
  fi

  log "installing Go $GO_VERSION to $GO_TARBALL_PREFIX/go ..."
  if [ -w "$GO_TARBALL_PREFIX" ]; then
    rm -rf "$GO_TARBALL_PREFIX/go"
    tar -C "$GO_TARBALL_PREFIX" -xzf "$TMP_DIR/$TARBALL"
  elif command -v sudo >/dev/null 2>&1; then
    sudo rm -rf "$GO_TARBALL_PREFIX/go"
    sudo mkdir -p "$GO_TARBALL_PREFIX"
    sudo tar -C "$GO_TARBALL_PREFIX" -xzf "$TMP_DIR/$TARBALL"
  else
    log "no write access to $GO_TARBALL_PREFIX and no sudo — installing to $LOCAL_GO_PREFIX/go"
    mkdir -p "$LOCAL_GO_PREFIX"
    rm -rf "$LOCAL_GO_PREFIX/go"
    tar -C "$LOCAL_GO_PREFIX" -xzf "$TMP_DIR/$TARBALL"
  fi

  GO="/usr/local/go/bin/go"
  [ -x "$GO" ] || GO="$LOCAL_GO_PREFIX/go/bin/go"
  go_satisfies "$GO" || die "installed Go does not satisfy go.mod requirement ($GO_VERSION)"
  log "installed: $("$GO" version | awk '{print $3}')"
fi

# --- build ------------------------------------------------------------------
log "building $APP_NAME (Go: $("$GO" version | awk '{print $3}')) ..."
mkdir -p "$BIN_DIR"
export CGO_ENABLED="${CGO_ENABLED:-0}"   # static binary; set CGO_ENABLED=1 to override
(
  cd "$SCRIPT_DIR"
  "$GO" build -trimpath -ldflags="-s -w" -o "$BIN_DIR/$APP_NAME" .
)

log "done → $BIN_DIR/$APP_NAME"
"$BIN_DIR/$APP_NAME" --version >/dev/null 2>&1 && log "binary runs OK" || log "note: binary built, but '--version' isn't supported"
