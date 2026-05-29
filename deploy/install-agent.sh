#!/usr/bin/env bash
# MCP Mesh — Agent install script
# Usage (on any Linux device or Termux on Android):
#   curl -sSL https://raw.github.com/mcp-mesh/agent/main/deploy/install-agent.sh | bash
#
# Or locally after cloning the repo:
#   bash deploy/install-agent.sh [device-name] [hub-ip]

set -euo pipefail

DEVICE_NAME="${1:-}"
HUB_IP="${2:-}"
INSTALL_DIR="${HOME}/.local/bin"
CONFIG_DIR="${HOME}/.config/mcp-mesh"

# ── Detect architecture ───────────────────────────────────────────────────────
ARCH="$(uname -m)"
case "${ARCH}" in
  x86_64)          BINARY="agent-linux-amd64" ;;
  aarch64|arm64)   BINARY="agent-linux-arm64" ;;
  armv7l|armhf)    BINARY="agent-linux-arm"   ;;
  *)
    echo "ERROR: Unsupported architecture: ${ARCH}"
    exit 1
    ;;
esac

echo ""
echo "MCP Mesh Agent Installer"
echo "  Architecture : ${ARCH} → ${BINARY}"
echo "  Install dir  : ${INSTALL_DIR}"
echo "  Config dir   : ${CONFIG_DIR}"
echo ""

# ── Install binary ────────────────────────────────────────────────────────────
mkdir -p "${INSTALL_DIR}" "${CONFIG_DIR}"

# In a real release, download from GitHub releases:
# curl -sSL "https://github.com/mcp-mesh/agent/releases/latest/download/${BINARY}" \
#   -o "${INSTALL_DIR}/mesh-agent"

# For local development, copy from dist/:
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
DIST_BIN="${SCRIPT_DIR}/../dist/${BINARY}"
if [ -f "${DIST_BIN}" ]; then
  cp "${DIST_BIN}" "${INSTALL_DIR}/mesh-agent"
else
  echo "ERROR: Binary not found at ${DIST_BIN}"
  echo "Run 'make build-all' first to build all platform binaries."
  exit 1
fi

chmod +x "${INSTALL_DIR}/mesh-agent"

# ── Write config if not exists ────────────────────────────────────────────────
CONFIG_FILE="${CONFIG_DIR}/agent.yml"
if [ ! -f "${CONFIG_FILE}" ]; then
  DEVICE_NAME="${DEVICE_NAME:-$(hostname -s)}"
  HUB_IP="${HUB_IP:-192.168.1.100}"

  cat > "${CONFIG_FILE}" << EOF
device_name: "${DEVICE_NAME}"
device_type: "pc"
secret: "REPLACE_WITH_HUB_SECRET"

mqtt:
  host: "${HUB_IP}"
  port: 1883
  username: "meshuser"
  password: "REPLACE_WITH_MQTT_PASSWORD"
  keepalive: 60

agent:
  host: "0.0.0.0"
  port: 7800
  transfer_port: 7801
  allowed_paths:
    - "${HOME}"
    - "/tmp"
  allow_run_command: false
  allow_destructive: true
EOF
  echo "Config written to ${CONFIG_FILE}"
  echo "IMPORTANT: Edit ${CONFIG_FILE} and set your secret and mqtt.password"
else
  echo "Config already exists at ${CONFIG_FILE} — skipping"
fi

echo ""
echo "Installation complete."
echo ""
echo "Next steps:"
echo "  1. Edit   ${CONFIG_FILE}"
echo "  2. Run    ${INSTALL_DIR}/mesh-agent --config ${CONFIG_FILE}"
echo ""
