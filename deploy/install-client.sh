#!/bin/sh
set -eu

REPO="wfu-work/navmesh-go"
VERSION="latest"
INSTALL_DIR="/opt/navmesh"
SERVICE_NAME="navmesh-client"
SERVER="navmesh.navfirst.com"
API="https://navmesh.navfirst.com"
PORT="3008"
TOKEN="navfirst@2020"
EXTRA_ARGS=""
DOWNLOAD_BASE=""

usage() {
  cat <<'EOF'
NavMesh client installer

Usage:
  sh install-client.sh [options]

Options:
  --repo OWNER/REPO          GitHub repository, default wfu-work/navmesh-go
  --version VERSION          Release tag, for example v0.0.2. Default latest
  --install-dir DIR          Install directory, default /opt/navmesh
  --service-name NAME        systemd service name, default navmesh-client
  --server HOST              NavMesh tunnel server, default navmesh.navfirst.com
  --api URL                  NavMesh API base URL, default https://navmesh.navfirst.com
  --port PORT                NavMesh tunnel UDP port, default 3008
  --token TOKEN              Bootstrap register token, default navfirst@2020
  --extra-args "ARGS"        Extra navmesh-client arguments
  --download-base URL        Custom binary download base URL, for private mirror/CDN
  -h, --help                 Show help

Example:
  curl -fsSL https://github.com/wfu-work/navmesh-go/releases/latest/download/install-client.sh | sudo sh -s -- \
    --server navmesh.navfirst.com \
    --api https://navmesh.navfirst.com \
    --token navfirst@2020

  curl -fsSL https://navmesh.navfirst.com/download/install-client.sh | sudo sh -s -- \
    --download-base https://navmesh.navfirst.com/download \
    --server navmesh.navfirst.com \
    --api https://navmesh.navfirst.com \
    --token navfirst@2020
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
    --repo)
      REPO="${2:-}"
      shift 2
      ;;
    --version)
      VERSION="${2:-}"
      shift 2
      ;;
    --install-dir)
      INSTALL_DIR="${2:-}"
      shift 2
      ;;
    --service-name)
      SERVICE_NAME="${2:-}"
      shift 2
      ;;
    --server)
      SERVER="${2:-}"
      shift 2
      ;;
    --api)
      API="${2:-}"
      shift 2
      ;;
    --port)
      PORT="${2:-}"
      shift 2
      ;;
    --token)
      TOKEN="${2:-}"
      shift 2
      ;;
    --extra-args)
      EXTRA_ARGS="${2:-}"
      shift 2
      ;;
    --download-base)
      DOWNLOAD_BASE="${2:-}"
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
[ -n "$REPO" ] || die "--repo is required"
[ -n "$INSTALL_DIR" ] || die "--install-dir is required"
[ -n "$SERVICE_NAME" ] || die "--service-name is required"
[ -n "$SERVER" ] || die "--server is required"
[ -n "$API" ] || die "--api is required"
[ -n "$PORT" ] || die "--port is required"
[ -n "$TOKEN" ] || die "--token is required"

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

need_cmd curl
need_cmd install
need_cmd systemctl

ASSET="navmesh-client-${GOOS}-${GOARCH}"
if [ -n "$DOWNLOAD_BASE" ]; then
  DOWNLOAD_URL="${DOWNLOAD_BASE%/}/${ASSET}"
elif [ "$VERSION" = "latest" ]; then
  DOWNLOAD_URL="https://github.com/${REPO}/releases/latest/download/${ASSET}"
else
  DOWNLOAD_URL="https://github.com/${REPO}/releases/download/${VERSION}/${ASSET}"
fi

TMP_DIR="$(mktemp -d)"
cleanup() {
  rm -rf "$TMP_DIR"
}
trap cleanup EXIT INT TERM

log "Downloading ${ASSET}"
curl -fL --retry 3 --connect-timeout 15 -o "${TMP_DIR}/navmesh-client" "$DOWNLOAD_URL"
[ -s "${TMP_DIR}/navmesh-client" ] || die "downloaded binary is empty"

log "Installing navmesh-client to ${INSTALL_DIR}/navmesh-client"
install -d -m 0755 "$INSTALL_DIR"
if systemctl is-active --quiet "$SERVICE_NAME" 2>/dev/null; then
  log "Stopping existing ${SERVICE_NAME} service"
  systemctl stop "$SERVICE_NAME"
fi
install -m 0755 "${TMP_DIR}/navmesh-client" "${INSTALL_DIR}/navmesh-client"
ln -sf "${INSTALL_DIR}/navmesh-client" /usr/local/bin/navmesh-client

SERVICE_FILE="/etc/systemd/system/${SERVICE_NAME}.service"
log "Writing systemd service ${SERVICE_FILE}"
cat > "$SERVICE_FILE" <<EOF
[Unit]
Description=NavMesh Edge Client
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
WorkingDirectory=${INSTALL_DIR}
ExecStart=${INSTALL_DIR}/navmesh-client -server ${SERVER} -api ${API} -port ${PORT} -token ${TOKEN} ${EXTRA_ARGS}
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
