#!/bin/sh
# Boo Boo installer for Debian / Ubuntu / Raspberry Pi OS.
# Usage: curl -fsSL https://raw.githubusercontent.com/fiaboo1628-pixel/boo-boo/main/scripts/install.sh | sudo sh
set -eu

REPO="fiaboo1628-pixel/boo-boo"

if [ "$(id -u)" -ne 0 ]; then
  echo "Please run as root (sudo)." >&2
  exit 1
fi

case "$(uname -m)" in
  x86_64|amd64) ARCH=amd64 ;;
  aarch64|arm64) ARCH=arm64 ;;
  armv7l|armv7) ARCH=armv7 ;;
  *) echo "Unsupported CPU: $(uname -m)" >&2; exit 1 ;;
esac

if ! command -v docker >/dev/null 2>&1; then
  echo "==> Installing Docker"
  curl -fsSL https://get.docker.com | sh
fi
systemctl enable --now docker

echo "==> Downloading Boo Boo ($ARCH)"
curl -fsSL -o /usr/local/bin/booboo \
  "https://github.com/$REPO/releases/latest/download/booboo-linux-$ARCH"
chmod +x /usr/local/bin/booboo

echo "==> Installing service"
curl -fsSL -o /etc/systemd/system/booboo.service \
  "https://raw.githubusercontent.com/$REPO/main/scripts/booboo.service"
mkdir -p /var/lib/booboo
systemctl daemon-reload
systemctl enable --now booboo

IP=$(hostname -I 2>/dev/null | awk '{print $1}')
echo
echo "Boo Boo is running: http://${IP:-<server-ip>}:8080"
echo "Open it in a browser and create your admin account."
