#!/bin/sh
# RaiseSteady Guard for Linux systems without a deb or rpm package manager
# (Arch, Manjaro and others with systemd). Run from the unpacked folder:
#   sudo ./install.sh
set -eu
[ "$(id -u)" = 0 ] || { echo "Please run this with sudo: sudo ./install.sh" >&2; exit 1; }
command -v systemctl >/dev/null 2>&1 || { echo "RaiseSteady Guard needs systemd." >&2; exit 1; }
HERE="$(cd "$(dirname "$0")" && pwd)"
D=/opt/raisesteady-guard
install -d -m 0755 "$D"
install -m 0755 "$HERE/raisesteady-guard" "$D/raisesteady-guard"
[ -f "$HERE/raisesteady-guard.xpi" ] && install -m 0644 "$HERE/raisesteady-guard.xpi" "$D/raisesteady-guard.xpi"
sed "s#/usr/lib/raisesteady-guard/raisesteady-guard#$D/raisesteady-guard#" "$HERE/open-parent-page" > "$D/open-parent-page"
chmod 0755 "$D/open-parent-page"
install -m 0755 "$HERE/uninstall.sh" "$D/uninstall.sh"
ln -sf "$D/raisesteady-guard" /usr/local/bin/raisesteady-guard
sed "s#/usr/lib/raisesteady-guard/raisesteady-guard#$D/raisesteady-guard#" "$HERE/raisesteady-guard.service" > /etc/systemd/system/raisesteady-guard.service
chmod 0644 /etc/systemd/system/raisesteady-guard.service
install -d -m 0755 /usr/local/share/applications /usr/local/share/icons/hicolor/128x128/apps
sed "s#/usr/lib/raisesteady-guard/open-parent-page#$D/open-parent-page#" "$HERE/raisesteady-guard.desktop" > /usr/local/share/applications/raisesteady-guard.desktop
install -m 0644 "$HERE/raisesteady-guard.png" /usr/local/share/icons/hicolor/128x128/apps/raisesteady-guard.png
systemctl daemon-reload
if [ -f /var/lib/raisesteady-guard/config.json ]; then
  systemctl try-restart raisesteady-guard.service || true
  echo "RaiseSteady Guard updated."
else
  echo "RaiseSteady Guard is installed. To finish, run:"
  echo "  sudo raisesteady-guard setup"
fi
