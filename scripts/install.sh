#!/bin/sh
# Boo Boo installer for Arch Linux, Debian, Ubuntu and Raspberry Pi OS.
# Usage: curl -fsSL https://raw.githubusercontent.com/fiaboo1628-pixel/boo-boo/main/scripts/install.sh | sudo sh
#
# Optional environment variables:
#   BOOBOO_AUDIO_USER  user whose PipeWire session plays music (default: the user who ran sudo)
#   BOOBOO_MUSIC_DIR   music folder (default: /srv/music)
#   BOOBOO_PORT        web UI port (default: 8080)
set -eu

REPO="fiaboo1628-pixel/boo-boo"
AUDIO_USER="${BOOBOO_AUDIO_USER:-${SUDO_USER:-}}"
MUSIC_DIR="${BOOBOO_MUSIC_DIR:-/srv/music}"
PORT="${BOOBOO_PORT:-8080}"

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

echo "==> Installing Docker, Bluetooth and audio packages"
if command -v pacman >/dev/null 2>&1; then
  # --needed skips anything already installed, and there is no -u, so
  # nothing else on the system gets upgraded.
  pacman -S --needed --noconfirm docker docker-compose bluez bluez-utils \
    pipewire pipewire-pulse wireplumber mpv
elif command -v apt-get >/dev/null 2>&1; then
  command -v docker >/dev/null 2>&1 || curl -fsSL https://get.docker.com | sh
  apt-get install -y bluez pipewire pipewire-pulse wireplumber libspa-0.2-bluetooth mpv
else
  echo "Unsupported distro: needs pacman or apt-get." >&2
  exit 1
fi
systemctl enable --now docker bluetooth

if [ -n "$AUDIO_USER" ] && id "$AUDIO_USER" >/dev/null 2>&1; then
  echo "==> Letting $AUDIO_USER's PipeWire run without a login (for the speaker)"
  loginctl enable-linger "$AUDIO_USER"
  AUDIO_UID=$(id -u "$AUDIO_USER")
  runuser -u "$AUDIO_USER" -- env XDG_RUNTIME_DIR="/run/user/$AUDIO_UID" \
    systemctl --user enable --now pipewire pipewire-pulse wireplumber || true
else
  echo "==> No audio user found; music playback will run as root."
  AUDIO_USER=""
fi

echo "==> Downloading Boo Boo ($ARCH)"
curl -fsSL -o /usr/local/bin/booboo \
  "https://github.com/$REPO/releases/latest/download/booboo-linux-$ARCH"
chmod +x /usr/local/bin/booboo

mkdir -p /var/lib/booboo "$MUSIC_DIR"
[ -n "$AUDIO_USER" ] && chown "$AUDIO_USER" "$MUSIC_DIR"

echo "==> Installing service"
cat > /etc/systemd/system/booboo.service <<UNIT
[Unit]
Description=Boo Boo home server dashboard
After=network-online.target docker.service bluetooth.service
Wants=network-online.target docker.service

[Service]
ExecStart=/usr/local/bin/booboo -addr :$PORT -data /var/lib/booboo -music $MUSIC_DIR -audio-user "$AUDIO_USER"
Restart=on-failure
# Keep the dashboard light so it never competes with other jobs on the box.
Nice=10
CPUWeight=20
MemoryMax=256M

[Install]
WantedBy=multi-user.target
UNIT
systemctl daemon-reload
systemctl enable --now booboo
systemctl restart booboo

IP=$(ip route get 1.1.1.1 2>/dev/null | awk '{for(i=1;i<NF;i++) if($i=="src") print $(i+1)}')
echo
echo "Boo Boo is running: http://${IP:-<server-ip>}:$PORT"
echo "Put music in $MUSIC_DIR, pair your speaker under Bluetooth, and press play."
