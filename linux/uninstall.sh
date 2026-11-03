#!/bin/sh
# Removes RaiseSteady Guard installed with install.sh: the browser settings,
# the service, its data and its files.
#   sudo /opt/raisesteady-guard/uninstall.sh
set -eu
[ "$(id -u)" = 0 ] || { echo "Please run this with sudo." >&2; exit 1; }
D=/opt/raisesteady-guard
if [ -x "$D/raisesteady-guard" ]; then
  "$D/raisesteady-guard" package-remove
fi
rm -f /etc/systemd/system/raisesteady-guard.service /usr/local/bin/raisesteady-guard \
  /usr/local/share/applications/raisesteady-guard.desktop \
  /usr/local/share/icons/hicolor/128x128/apps/raisesteady-guard.png
systemctl daemon-reload || true
rm -rf "$D"
echo "RaiseSteady Guard removed."
