#!/bin/sh
# RaiseSteady Guard package: after install or upgrade (deb: configure; rpm: $1 = 1 or 2).
# It asks nothing. The program writes the safe browser settings, enables and
# starts the service, and on a new install opens the parent page for the person
# at the screen: the first-run setup (terms, age, PIN) is on that page.
case "${1:-configure}" in
  configure|1|2) ;;
  *) exit 0 ;;
esac
if command -v update-desktop-database >/dev/null 2>&1; then update-desktop-database -q /usr/share/applications >/dev/null 2>&1 || true; fi
if command -v gtk-update-icon-cache >/dev/null 2>&1; then gtk-update-icon-cache -q -t /usr/share/icons/hicolor >/dev/null 2>&1 || true; fi
# A failure here must not leave the package half-installed: say what happened
# and let the package manager finish.
/usr/lib/raisesteady-guard/raisesteady-guard package-install || {
  echo "RaiseSteady Guard: the last install step did not finish (see above)." >&2
  echo "Restart the computer. If the parent page still does not open from the app menu, install the package again." >&2
}
exit 0
