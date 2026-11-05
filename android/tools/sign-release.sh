#!/usr/bin/env bash
# RaiseSteady Guard for Android: the signed release APK (design 7), on the owner's Mac.
# The key lives outside the repository: ~/Developer/keys/guard-android/guard-release.p12, its
# passwords in passwords.env next to it (mode 600; GUARD_KEYSTORE, GUARD_KEYSTORE_PASSWORD,
# GUARD_KEY_ALIAS, GUARD_KEY_PASSWORD). The passwords are read into the build's environment only,
# never printed. Prints the APK's path, its SHA-256 and the certificate's SHA-256 fingerprint
# (public: it goes into the release notes).
#
#   bash kidguard/android/tools/sign-release.sh        (GUARD_KEYS_ENV=<file> for another key)
#
# Needs JDK 17 (JAVA_HOME) and the Android SDK (ANDROID_HOME) with build tools.
set -euo pipefail
set +x

AND="$(cd "$(dirname "$0")/.." && pwd)"
KEYS="${GUARD_KEYS_ENV:-$HOME/Developer/keys/guard-android/passwords.env}"
fail() { echo "sign-release: $*" >&2; exit 1; }

[ -f "$KEYS" ] || fail "no key settings at $KEYS (make the key first: kidguard/DEVELOPING.md, Android)"
mode="$(stat -f '%Lp' "$KEYS" 2>/dev/null || stat -c '%a' "$KEYS")"
[ "$mode" = 600 ] || fail "$KEYS must be mode 600 (it is $mode)"
[ -n "${ANDROID_HOME:-}" ] || fail "ANDROID_HOME is not set"

# The four GUARD_* values, for Gradle only (app/build.gradle.kts signs when all four are set).
set -a
# shellcheck disable=SC1090
. "$KEYS"
set +a
for v in GUARD_KEYSTORE GUARD_KEYSTORE_PASSWORD GUARD_KEY_ALIAS GUARD_KEY_PASSWORD; do
  [ -n "${!v:-}" ] || fail "$v is missing in $KEYS"
done
[ -f "$GUARD_KEYSTORE" ] || fail "no keystore at $GUARD_KEYSTORE"

version="$(sed -n 's/^versionName=//p' "$AND/version.properties")"
apk="$AND/app/build/outputs/apk/release/raisesteady-guard-android-$version.apk"
rm -f "$apk"
( cd "$AND" && ./gradlew --no-daemon -q :app:assembleRelease )
[ -f "$apk" ] || fail "the build made no $apk"

bt="$(ls -d "$ANDROID_HOME"/build-tools/* | sort -V | tail -1)"
certs="$("$bt/apksigner" verify --print-certs "$apk")" || fail "apksigner verify failed"
fp="$(sed -n 's/^Signer #1 certificate SHA-256 digest: //p' <<< "$certs")"
[ -n "$fp" ] || fail "no signer certificate in $apk"
( cd "$(dirname "$apk")" && shasum -a 256 "$(basename "$apk")" > "$(basename "$apk").sha256" )

echo "APK: $apk"
echo "APK SHA-256: $(cut -d' ' -f1 < "$apk.sha256")"
echo "apksigner verify: passed"
echo "Certificate SHA-256: $fp"
