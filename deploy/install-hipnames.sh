#!/bin/sh
set -eu

INSTALL_DIR="/mnt/navfirst/nav-hipnames"
SERVICE_NAME="hipnames"
DOWNLOAD_BASE=""
RELEASE_TYPE="hipnames"
DEVICE_TYPE="hipnames"
APP_NAME=""
EXE_FILE=""

usage() {
  cat <<'EOF'
NavFirst hipnames installer

Usage:
  sh install-hipnames.sh [options]

Options:
  --install-dir DIR          Install directory, default /mnt/navfirst/nav-hipnames
  --service-name NAME        systemd service name, default hipnames
  --download-base URL        NavMesh downloads API base URL, for example https://navmesh.navfirst.com/api/downloads
  --release-type TYPE        Release type, default hipnames
  --device-type TYPE         Device type, default hipnames
  --app-name NAME            Executable name in package, auto-detect by default
  --exe-file FILE            Local executable for offline installation
  -h, --help                 Show help

Example:
  curl -fsSL https://navmesh.navfirst.com/api/downloads/install-hipnames.sh | sudo sh -s -- \
    --download-base https://navmesh.navfirst.com/api/downloads

  sudo ./install-hipnames.sh --exe-file ./navHipnames
EOF
}

log() {
  printf '%s\n' "==> $*"
}

die() {
  printf '%s\n' "ERROR: $*" >&2
  exit 1
}

need_cmd() {
  command -v "$1" >/dev/null 2>&1 || die "missing command: $1"
}

while [ "$#" -gt 0 ]; do
  case "$1" in
    --install-dir)
      INSTALL_DIR="${2:-}"
      shift 2
      ;;
    --service-name)
      SERVICE_NAME="${2:-}"
      shift 2
      ;;
    --download-base)
      DOWNLOAD_BASE="${2:-}"
      shift 2
      ;;
    --release-type)
      RELEASE_TYPE="${2:-}"
      shift 2
      ;;
    --device-type)
      DEVICE_TYPE="${2:-}"
      shift 2
      ;;
    --app-name)
      APP_NAME="${2:-}"
      shift 2
      ;;
    --exe-file)
      EXE_FILE="${2:-}"
      shift 2
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      die "unknown option: $1"
      ;;
  esac
done

[ "$(id -u)" -eq 0 ] || die "please run as root, for example: curl ... | sudo sh"
[ -n "$INSTALL_DIR" ] || die "--install-dir is required"
[ -n "$SERVICE_NAME" ] || die "--service-name is required"
[ -n "$DOWNLOAD_BASE" ] || [ -n "$EXE_FILE" ] || die "--download-base is required unless --exe-file is provided"
[ -n "$RELEASE_TYPE" ] || die "--release-type is required"
[ -n "$DEVICE_TYPE" ] || die "--device-type is required"
[ -z "$EXE_FILE" ] || [ -f "$EXE_FILE" ] || die "--exe-file not found: $EXE_FILE"

OS="$(uname -s | tr '[:upper:]' '[:lower:]')"
ARCH="$(uname -m)"

case "$OS" in
  linux)
    GOOS="linux"
    ;;
  *)
    die "unsupported OS: $OS. This installer currently supports Linux systemd hosts."
    ;;
esac

case "$ARCH" in
  x86_64|amd64)
    GOARCH="amd64"
    ;;
  aarch64|arm64)
    GOARCH="arm64"
    ;;
  *)
    die "unsupported architecture: $ARCH"
    ;;
esac

if [ -z "$EXE_FILE" ]; then
  need_cmd curl
fi
need_cmd find
need_cmd install
need_cmd systemctl

TMP_DIR="$(mktemp -d)"
cleanup() {
  rm -rf "$TMP_DIR"
}
trap cleanup EXIT INT TERM

DOWNLOAD_URL="${DOWNLOAD_BASE%/}/releases/latest?releaseType=${RELEASE_TYPE}&deviceType=${DEVICE_TYPE}&os=${GOOS}&arch=${GOARCH}"
DOWNLOAD_PATH="${TMP_DIR}/hipnames-download"
EXTRACT_DIR="${TMP_DIR}/extract"
OFFLINE_APP_PATH=""

mkdir -p "$EXTRACT_DIR"

if [ -n "$EXE_FILE" ]; then
  APP_FILE="${APP_NAME:-$(basename "$EXE_FILE")}"
  OFFLINE_APP_PATH="${EXTRACT_DIR}/${APP_FILE}"
  log "Using local executable ${EXE_FILE}"
  cp "$EXE_FILE" "$OFFLINE_APP_PATH"
else
  log "Downloading latest ${RELEASE_TYPE} file for ${GOOS}/${GOARCH}"
  curl -fL --retry 3 --connect-timeout 15 -o "$DOWNLOAD_PATH" "$DOWNLOAD_URL"
  [ -s "$DOWNLOAD_PATH" ] || die "downloaded file is empty"

  if command -v unzip >/dev/null 2>&1 && unzip -t "$DOWNLOAD_PATH" >/dev/null 2>&1; then
    log "Extracting zip package"
    unzip -q "$DOWNLOAD_PATH" -d "$EXTRACT_DIR"
  elif tar -tzf "$DOWNLOAD_PATH" >/dev/null 2>&1; then
    log "Extracting tar.gz package"
    tar -xzf "$DOWNLOAD_PATH" -C "$EXTRACT_DIR"
  else
    log "Treating downloaded file as binary"
    cp "$DOWNLOAD_PATH" "$EXTRACT_DIR/${APP_NAME:-hipnames}"
  fi
fi

find_app() {
  if [ -n "$APP_NAME" ]; then
    find "$EXTRACT_DIR" -type f -name "$APP_NAME" | head -n 1
    return
  fi
  for name in navHipnames nav-hipnames hipnames hipnamesApp navHipnamesApp; do
    found="$(find "$EXTRACT_DIR" -type f -name "$name" | head -n 1)"
    if [ -n "$found" ]; then
      printf '%s\n' "$found"
      return
    fi
  done
  find "$EXTRACT_DIR" -type f -perm -111 | head -n 1
}

if [ -n "$OFFLINE_APP_PATH" ]; then
  APP_PATH="$OFFLINE_APP_PATH"
else
  APP_PATH="$(find_app)"
fi
[ -n "$APP_PATH" ] || die "hipnames executable not found; use --app-name to specify it"
APP_FILE="$(basename "$APP_PATH")"
APP_DIR="$(dirname "$APP_PATH")"

log "Installing hipnames application to ${INSTALL_DIR}"
install -d -m 0755 "$INSTALL_DIR"
if systemctl is-active --quiet "$SERVICE_NAME" 2>/dev/null; then
  log "Stopping existing ${SERVICE_NAME} service"
  systemctl stop "$SERVICE_NAME"
fi
cp -R "$APP_DIR"/. "$INSTALL_DIR"/
chmod +x "$INSTALL_DIR/$APP_FILE"

SERVICE_FILE="/etc/systemd/system/${SERVICE_NAME}.service"
log "Writing systemd service ${SERVICE_FILE}"
cat > "$SERVICE_FILE" <<EOF
[Unit]
Description=NavFirst Hipnames
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
WorkingDirectory=${INSTALL_DIR}
ExecStart=${INSTALL_DIR}/${APP_FILE}
Restart=always
RestartSec=5
LimitNOFILE=65535

[Install]
WantedBy=multi-user.target
EOF

log "Starting ${SERVICE_NAME} service"
systemctl daemon-reload
systemctl enable "$SERVICE_NAME"
systemctl restart "$SERVICE_NAME"

log "Installed successfully"
systemctl --no-pager --full status "$SERVICE_NAME" || true
