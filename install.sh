#!/usr/bin/env bash

set -euo pipefail

BINARY_NAME_DEFAULT="qoder-github-mcp-server"
BASE_URL_DEFAULT="https://download.qoder.com/qodercli/mcp/qoder-github-mcp-server/releases"

BINARY_NAME="${BINARY_NAME:-$BINARY_NAME_DEFAULT}"
BASE_URL="${BASE_URL:-$BASE_URL_DEFAULT}"
VERSION="${VERSION:-}"
OS_OVERRIDE=""
ARCH_OVERRIDE=""
INSTALL_DIR_ENV="${INSTALL_DIR:-}"
INSTALL_DIR_ARG=""
FORCE_INSTALL=0
SKIP_VERIFY=0

usage() {
  cat <<'EOF'
install.sh - download and install qoder-github-mcp-server

Usage:
  ./install.sh --version <version> [options]

Required arguments:
  -v, --version <version>     Version to install (e.g. 0.2.0)

Optional arguments:
      --os <os>               Override OS detection (darwin|linux|windows)
      --arch <arch>           Override arch detection (amd64|arm64)
  -i, --install-dir <dir>     Installation directory (default: $HOME/.local/bin)
  -f, --force                 Overwrite an existing binary
      --skip-verify           Skip SHA256 verification (not recommended)
      --base-url <url>        Override the download base URL
  -h, --help                  Show this help message

Environment variables:
  VERSION, BASE_URL, INSTALL_DIR, BINARY_NAME can all override defaults.
EOF
}

err() {
  echo "[ERROR] $*" >&2
}

info() {
  echo "[INFO] $*"
}

have_cmd() {
  command -v "$1" >/dev/null 2>&1
}

require_cmd() {
  if ! have_cmd "$1"; then
    err "Command '$1' is required. Please install it first."
    exit 1
  fi
}

download_file() {
  local url="$1"
  local dest="$2"

  if have_cmd curl; then
    curl -fL --progress-bar -o "$dest" "$url"
  elif have_cmd wget; then
    wget -q -O "$dest" "$url"
  else
    err "Neither curl nor wget is available, cannot download."
    exit 1
  fi
}

detect_os() {
  local override="$1"
  local uname_out

  if [ -n "$override" ]; then
    echo "$override"
    return
  fi

  uname_out="$(uname -s 2>/dev/null || echo unknown)"
  case "$uname_out" in
    Darwin) echo "darwin" ;;
    Linux) echo "linux" ;;
    CYGWIN*|MINGW*|MSYS*|Windows_NT) echo "windows" ;;
    *) err "Unsupported operating system: $uname_out"; exit 1 ;;
  esac
}

detect_arch() {
  local override="$1"
  local uname_out

  if [ -n "$override" ]; then
    echo "$override"
    return
  fi

  uname_out="$(uname -m 2>/dev/null || echo unknown)"
  case "$uname_out" in
    x86_64|amd64) echo "amd64" ;;
    arm64|aarch64) echo "arm64" ;;
    *) err "Unsupported CPU architecture: $uname_out"; exit 1 ;;
  esac
}

checksum_cmd() {
  if have_cmd shasum; then
    echo "shasum -a 256"
  elif have_cmd sha256sum; then
    echo "sha256sum"
  else
    echo ""
  fi
}

verify_checksum() {
  local artifact="$1"
  local archive_path="$2"
  local version="$3"

  local tool
  tool="$(checksum_cmd)"
  if [ -z "$tool" ]; then
    info "Could not find shasum/sha256sum, skipping verification."
    return
  fi

  local sums_url="${BASE_URL}/${version}/SHA256SUMS"
  local sums_file="$TMP_DIR/SHA256SUMS"
  local downloaded=0
  if have_cmd curl; then
    if curl -fsSL -o "$sums_file" "$sums_url" >/dev/null 2>&1; then
      downloaded=1
    fi
  fi
  if [ "$downloaded" -ne 1 ] && have_cmd wget; then
    if wget -q -O "$sums_file" "$sums_url" >/dev/null 2>&1; then
      downloaded=1
    fi
  fi
  if [ "$downloaded" -ne 1 ]; then
    info "Could not download SHA256SUMS, skipping verification."
    return
  fi

  local expected_line
  expected_line="$(grep "  ${artifact}$" "$sums_file" || true)"
  if [ -z "$expected_line" ]; then
    info "No checksum found for ${artifact}, skipping verification."
    return
  fi

  local expected
  expected="$(printf "%s" "$expected_line" | awk '{print $1}')"
  local actual
  actual="$(eval "$tool \"$archive_path\"" | awk '{print $1}')"

  if [ "$expected" != "$actual" ]; then
    err "SHA256 verification failed: expected ${expected}, got ${actual}"
    exit 1
  fi

  info "SHA256 verification passed."
}

TMP_DIR=""
cleanup() {
  if [ -n "$TMP_DIR" ] && [ -d "$TMP_DIR" ]; then
    rm -rf "$TMP_DIR"
  fi
}
trap cleanup EXIT

while [ $# -gt 0 ]; do
  case "$1" in
    -v|--version)
      VERSION="$2"
      shift 2
      ;;
    --version=*)
      VERSION="${1#*=}"
      shift
      ;;
    --os)
      OS_OVERRIDE="$2"
      shift 2
      ;;
    --arch)
      ARCH_OVERRIDE="$2"
      shift 2
      ;;
    -i|--install-dir)
      INSTALL_DIR_ARG="$2"
      shift 2
      ;;
    --install-dir=*)
      INSTALL_DIR_ARG="${1#*=}"
      shift
      ;;
    -f|--force)
      FORCE_INSTALL=1
      shift
      ;;
    --skip-verify)
      SKIP_VERIFY=1
      shift
      ;;
    --base-url)
      BASE_URL="$2"
      shift 2
      ;;
    --base-url=*)
      BASE_URL="${1#*=}"
      shift
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      err "Unknown argument: $1"
      usage
      exit 1
      ;;
  esac
done

if [ -z "$VERSION" ]; then
  err "You must provide a version via --version or the VERSION env var."
  exit 1
fi

OS="$(detect_os "$OS_OVERRIDE")"
ARCH="$(detect_arch "$ARCH_OVERRIDE")"

if [ -n "$INSTALL_DIR_ARG" ]; then
  INSTALL_DIR="$INSTALL_DIR_ARG"
elif [ -n "$INSTALL_DIR_ENV" ]; then
  INSTALL_DIR="$INSTALL_DIR_ENV"
else
  INSTALL_DIR="$HOME/.local/bin"
fi

if [ ! -d "$INSTALL_DIR" ]; then
  info "Creating install directory: $INSTALL_DIR"
  if ! mkdir -p "$INSTALL_DIR" 2>/dev/null; then
    err "Failed to create ${INSTALL_DIR}. Try running with sudo or specify --install-dir."
    exit 1
  fi
fi

if [ ! -w "$INSTALL_DIR" ]; then
  err "No write permission for ${INSTALL_DIR}. Try running with sudo or specify --install-dir."
  exit 1
fi

TMP_DIR="$(mktemp -d)"

ARCHIVE_BASENAME="${BINARY_NAME}-${VERSION}-${OS}-${ARCH}"
ARCHIVE_EXT="tar.gz"
BINARY_FILENAME="$BINARY_NAME"

if [ "$OS" = "windows" ]; then
  ARCHIVE_EXT="zip"
  BINARY_FILENAME="${BINARY_NAME}.exe"
fi

ARCHIVE_NAME="${ARCHIVE_BASENAME}.${ARCHIVE_EXT}"
DOWNLOAD_URL="${BASE_URL}/${VERSION}/${ARCHIVE_NAME}"
ARCHIVE_PATH="${TMP_DIR}/${ARCHIVE_NAME}"

info "Downloading ${DOWNLOAD_URL}"
download_file "$DOWNLOAD_URL" "$ARCHIVE_PATH"

if [ "$SKIP_VERIFY" -ne 1 ]; then
  verify_checksum "$ARCHIVE_NAME" "$ARCHIVE_PATH" "$VERSION"
else
  info "Skipping SHA256 verification."
fi

EXTRACT_DIR="${TMP_DIR}/extracted"
mkdir -p "$EXTRACT_DIR"

if [ "$ARCHIVE_EXT" = "zip" ]; then
  require_cmd unzip
  unzip -q "$ARCHIVE_PATH" -d "$EXTRACT_DIR"
else
  require_cmd tar
  tar -xzf "$ARCHIVE_PATH" -C "$EXTRACT_DIR"
fi

BUNDLE_DIR="${EXTRACT_DIR}/${ARCHIVE_BASENAME}"
if [ ! -d "$BUNDLE_DIR" ]; then
  BUNDLE_DIR="$(find "$EXTRACT_DIR" -mindepth 1 -maxdepth 1 -type d | head -n 1 || true)"
fi

if [ -z "$BUNDLE_DIR" ] || [ ! -d "$BUNDLE_DIR" ]; then
  err "Could not locate the extracted directory."
  exit 1
fi

SOURCE_BIN="${BUNDLE_DIR}/${BINARY_FILENAME}"
if [ ! -f "$SOURCE_BIN" ]; then
  err "Binary ${BINARY_FILENAME} not found in extracted archive."
  exit 1
fi

TARGET_PATH="${INSTALL_DIR}/${BINARY_FILENAME}"
if [ -f "$TARGET_PATH" ] && [ "$FORCE_INSTALL" -ne 1 ]; then
  err "${TARGET_PATH} already exists. Use --force to overwrite or specify --install-dir."
  exit 1
fi

if have_cmd install; then
  install -m 755 "$SOURCE_BIN" "$TARGET_PATH"
else
  cp "$SOURCE_BIN" "$TARGET_PATH"
  chmod 755 "$TARGET_PATH"
fi

info "Installed to ${TARGET_PATH}"

case ":$PATH:" in
  *:"$INSTALL_DIR":*)
    ;;
  *)
    info "Note: ${INSTALL_DIR} is not on PATH. Add it to run ${BINARY_FILENAME} directly."
    ;;
esac

info "Installation complete. Try: ${BINARY_FILENAME} --help"

