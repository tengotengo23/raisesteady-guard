#!/bin/sh
# RaiseSteady Guard package: after removal (deb: remove or purge; rpm: $1 = 0).
if command -v systemctl >/dev/null 2>&1; then systemctl daemon-reload >/dev/null 2>&1 || true; fi
exit 0
