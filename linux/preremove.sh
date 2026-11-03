#!/bin/sh
# RaiseSteady Guard package: before removal. deb: $1 = remove, purge or upgrade;
# rpm: $1 = 0 (removal) or 1 (upgrade). On removal the browser settings are
# taken out and the service stopped; on an upgrade nothing changes.
case "$1" in
  remove|purge|0)
    if [ -x /usr/lib/raisesteady-guard/raisesteady-guard ]; then
      # On failure the package stays (exit 1): browser settings left without
      # the service could not be taken out any more.
      /usr/lib/raisesteady-guard/raisesteady-guard package-remove || {
        echo "RaiseSteady Guard: could not remove every browser setting (see above). Fix that, then remove the package again." >&2
        exit 1
      }
    fi
    ;;
esac
exit 0
