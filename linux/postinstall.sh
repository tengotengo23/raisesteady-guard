#!/bin/sh
# RaiseSteady Guard package: after install or upgrade (deb: configure; rpm: $1 = 1 or 2).
if command -v systemctl >/dev/null 2>&1; then systemctl daemon-reload >/dev/null 2>&1 || true; fi
if [ -f /var/lib/raisesteady-guard/config.json ]; then
  # An upgrade: keep the settings, run the new version.
  systemctl try-restart raisesteady-guard.service >/dev/null 2>&1 || true
else
  echo ""
  echo "RaiseSteady Guard is installed. To finish, run:"
  echo "  sudo raisesteady-guard setup"
  echo ""
fi
if command -v update-desktop-database >/dev/null 2>&1; then update-desktop-database -q /usr/share/applications >/dev/null 2>&1 || true; fi
if command -v gtk-update-icon-cache >/dev/null 2>&1; then gtk-update-icon-cache -q -t /usr/share/icons/hicolor >/dev/null 2>&1 || true; fi
exit 0
