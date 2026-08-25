#!/bin/sh
set -eu

INSTALL_DIR="/mnt/navfirst/nav-rain-go"
SERVICE_NAME="raind"
DOWNLOAD_BASE=""
RELEASE_TYPE="rain"
DEVICE_TYPE="rain"
SKIP_DEPS="false"
EXE_FILE=""

usage() {
  cat <<'EOF'
NavFirst rain installer

Usage:
  sh install-rain.sh [options]

Options:
  --install-dir DIR          Install directory, default /mnt/navfirst/nav-rain-go
  --service-name NAME        systemd service name, default raind
  --download-base URL        NavMesh downloads API base URL, for example https://navmesh.navfirst.com/api/downloads
  --release-type TYPE        Release type, default rain
  --device-type TYPE         Device type, default rain
  --exe-file FILE            Local navRainApp executable for offline installation
  --skip-deps                Skip apt and pip dependency installation
  -h, --help                 Show help

Example:
  curl -fsSL https://navmesh.navfirst.com/api/downloads/install-rain.sh | sudo sh -s -- \
    --download-base https://navmesh.navfirst.com/api/downloads \
    --device-type rain

  sudo ./install-rain.sh --exe-file ./navRainApp --skip-deps
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
    --exe-file)
      EXE_FILE="${2:-}"
      shift 2
      ;;
    --skip-deps)
      SKIP_DEPS="true"
      shift
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
need_cmd install
need_cmd systemctl

install_dependencies() {
  if [ "$SKIP_DEPS" = "true" ]; then
    log "Skipping dependency installation"
    return
  fi
  if ! command -v apt-get >/dev/null 2>&1; then
    log "apt-get not found, skipping system dependency installation"
    return
  fi
  log "Installing system dependencies"
  apt-get update
  apt-get install -y python3-pip unzip lrzsz
  mkdir -p "$HOME/.pip"
  cat > "$HOME/.pip/pip.conf" <<'EOF'
[global]
index-url = https://mirrors.aliyun.com/pypi/simple/
[install]
trusted-host = mirrors.aliyun.com
EOF
  if command -v pip3 >/dev/null 2>&1; then
    log "Installing Python dependencies"
  fi
}

TMP_DIR="$(mktemp -d)"
cleanup() {
  rm -rf "$TMP_DIR"
}
trap cleanup EXIT INT TERM

DOWNLOAD_URL="${DOWNLOAD_BASE%/}/releases/latest?releaseType=${RELEASE_TYPE}&deviceType=${DEVICE_TYPE}&os=${GOOS}&arch=${GOARCH}"
DOWNLOAD_PATH="${TMP_DIR}/rain-download"
EXTRACT_DIR="${TMP_DIR}/extract"

install_dependencies
mkdir -p "$EXTRACT_DIR"

if [ -n "$EXE_FILE" ]; then
  log "Using local executable ${EXE_FILE}"
  cp "$EXE_FILE" "$EXTRACT_DIR/navRainApp"
else
  log "Downloading latest ${RELEASE_TYPE} file for ${GOOS}/${GOARCH}"
  curl -fL --retry 3 --connect-timeout 15 -o "$DOWNLOAD_PATH" "$DOWNLOAD_URL"
  [ -s "$DOWNLOAD_PATH" ] || die "downloaded file is empty"

  if unzip -t "$DOWNLOAD_PATH" >/dev/null 2>&1; then
    log "Extracting zip package"
    unzip -q "$DOWNLOAD_PATH" -d "$EXTRACT_DIR"
  elif tar -tzf "$DOWNLOAD_PATH" >/dev/null 2>&1; then
    log "Extracting tar.gz package"
    tar -xzf "$DOWNLOAD_PATH" -C "$EXTRACT_DIR"
  else
    log "Treating downloaded file as navRainApp binary"
    cp "$DOWNLOAD_PATH" "$EXTRACT_DIR/navRainApp"
  fi
fi

APP_PATH="$(find "$EXTRACT_DIR" -type f -name navRainApp | head -n 1)"
[ -n "$APP_PATH" ] || die "navRainApp not found"
APP_DIR="$(dirname "$APP_PATH")"

log "Installing rain application to ${INSTALL_DIR}"
install -d -m 0755 "$INSTALL_DIR"
if systemctl is-active --quiet "$SERVICE_NAME" 2>/dev/null; then
  log "Stopping existing ${SERVICE_NAME} service"
  systemctl stop "$SERVICE_NAME"
fi
cp -R "$APP_DIR"/. "$INSTALL_DIR"/
chmod +x "$INSTALL_DIR/navRainApp"

SERVICE_FILE="/etc/systemd/system/${SERVICE_NAME}.service"
if [ -e "$SERVICE_FILE" ]; then
  log "Systemd service ${SERVICE_FILE} already exists; keeping existing file"
else
  log "Writing systemd service ${SERVICE_FILE}"
  cat > "$SERVICE_FILE" <<EOF
[Unit]
Description=navfirst rain predict for go
After=network.target

[Service]
Type=idle
TimeoutStartSec=infinity
ExecStartPre=/bin/sleep 5
ExecStart=${INSTALL_DIR}/navRainApp
WorkingDirectory=${INSTALL_DIR}
Restart=always
RestartSec=10s

[Install]
WantedBy=multi-user.target
EOF
fi

log "Starting ${SERVICE_NAME} service"
systemctl daemon-reload
systemctl enable "$SERVICE_NAME"
systemctl restart "$SERVICE_NAME"

log "Installed successfully"
systemctl --no-pager --full status "$SERVICE_NAME" || true
