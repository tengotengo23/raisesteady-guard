# Signed Firefox add-on

`raisesteady-guard-firefox.xpi` is the Firefox build of `ext/` (tools/build-ext.mjs), signed by Mozilla
(addons.mozilla.org, unlisted: signed, not listed in their catalogue). Firefox installs only signed add-ons,
so the packages ship this file: Linux at /usr/lib/raisesteady-guard/raisesteady-guard.xpi, Windows next to
the agent. The builds (tools/build-linux.sh, tools/build-installer.sh) copy it to dist/raisesteady-guard.xpi.

To sign a new version: raise the version in ext/manifest.json, build, then
`web-ext sign --channel=unlisted --source-dir dist/firefox` with the maintainer's AMO API key
(WEB_EXT_API_KEY, WEB_EXT_API_SECRET), and replace the file here. Each version can be signed once.
Current: 0.2.2.
