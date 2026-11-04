#!/usr/bin/env bash
# RaiseSteady Guard for Android: end-to-end tests on a booted emulator (design 6.2).
# CI runs it from ReactiveCircus/android-emulator-runner; it works on any device adb sees.
#
#   E2E_OUT=<dir> E2E_CLASSES="FilterTest GameTimeTest GuardTest" bash android/tools/e2e.sh
#
# Needs the APKs built first (./gradlew :app:assembleDebug :e2e:assembleDebug
# :e2e:assembleDebugAndroidTest). Writes to $E2E_OUT/<api>/: started (the emulator booted),
# instrument-<Class>.txt (raw results), results.txt (one line per test), screens/ (from the
# tests), logcat.txt, and passed when every test passed and logcat shows no crash.
# Tests are never retried: a failure is a failure.
set -uo pipefail

AND="$(cd "$(dirname "$0")/.." && pwd)"
CLASSES="${E2E_CLASSES:-FilterTest GameTimeTest GuardTest}"
PKG=com.raisesteady.guard
E2E=com.raisesteady.guard.e2e
RUNNER="$E2E.test/androidx.test.runner.AndroidJUnitRunner"
DEV_DIR=/data/local/tmp/guard-e2e

adb wait-for-device
API="$(adb shell getprop ro.build.version.sdk | tr -d '\r')"
OUT="${E2E_OUT:-$AND/build/e2e}/$API"
rm -rf "$OUT"; mkdir -p "$OUT/screens"
# The emulator booted and this script runs: from here on a failure is a test failure, not a boot failure.
date -u +%FT%TZ > "$OUT/started"
echo "API $API, $(adb shell getprop ro.product.model | tr -d '\r'), $(adb shell getprop ro.build.fingerprint | tr -d '\r')"

fail() { echo "e2e: $*"; adb logcat -d > "$OUT/logcat.txt" 2>/dev/null; exit 1; }

# Wait for the package manager and the settings provider (boot_completed comes a little early).
for i in $(seq 1 60); do
  adb shell pm path android >/dev/null 2>&1 && [ "$(adb shell getprop sys.boot_completed | tr -d '\r')" = 1 ] && break
  sleep 2
done
adb shell input keyevent 82 >/dev/null 2>&1 || true   # unlock the screen
adb shell svc power stayon true >/dev/null 2>&1 || true
adb shell settings put system screen_off_timeout 1800000 >/dev/null 2>&1 || true
adb logcat -c || true

APP_APK="$AND/app/build/outputs/apk/debug/app-debug.apk"
E2E_APK="$AND/e2e/build/outputs/apk/debug/e2e-debug.apk"
TEST_APK="$AND/e2e/build/outputs/apk/androidTest/debug/e2e-debug-androidTest.apk"
for a in "$APP_APK" "$E2E_APK" "$TEST_APK"; do [ -f "$a" ] || fail "missing $a (build it first)"; done

# A fresh install each run (an old device admin would refuse the uninstall: switch it off first).
adb shell dpm remove-active-admin "$PKG/.guard.GuardAdminReceiver" >/dev/null 2>&1 || true
adb uninstall "$PKG" >/dev/null 2>&1 || true
for a in "$APP_APK" "$E2E_APK" "$TEST_APK"; do
  adb install -r -g "$a" > "$OUT/install.txt" 2>&1 || { cat "$OUT/install.txt"; fail "install $a"; }
done

# Preparation as the shell user (design 6.2): VPN consent and usage access pre-approved,
# notifications allowed. Accessibility and the device admin are switched on by the tests.
adb shell appops set "$PKG" ACTIVATE_VPN allow || fail "appops ACTIVATE_VPN"
adb shell appops set "$PKG" GET_USAGE_STATS allow || fail "appops GET_USAGE_STATS"
if [ "$API" -ge 33 ]; then
  adb shell pm grant "$PKG" android.permission.POST_NOTIFICATIONS || fail "grant POST_NOTIFICATIONS"
fi
# Chrome without its first-run screens (read from this file when Chrome is the debug app).
if adb shell pm path com.android.chrome >/dev/null 2>&1; then
  adb shell 'echo "chrome --disable-fre --no-default-browser-check --no-first-run" > /data/local/tmp/chrome-command-line'
  adb shell am set-debug-app --persistent com.android.chrome >/dev/null 2>&1 || true
fi
adb shell rm -rf "$DEV_DIR"; adb shell mkdir -p "$DEV_DIR"

# One instrumentation per test class, in order. `am instrument` exits 0 even when tests fail,
# so the result codes are read from its raw output (-r): 0 ok, -1 error, -2 failure,
# -3 ignored, -4 assumption failed (counted as skipped, with its reason).
ok=1
: > "$OUT/results.txt"
for c in $CLASSES; do
  f="$OUT/instrument-$c.txt"
  echo "== $c"
  adb shell am instrument -w -r -e class "$E2E.$c" "$RUNNER" > "$f" 2>&1
  awk -v cls="$c" '
    /^INSTRUMENTATION_STATUS: test=/ { t = substr($0, index($0, "=") + 1) }
    /^INSTRUMENTATION_STATUS_CODE: / {
      code = $2
      if (code == 0) { print "PASS " cls "#" t; n++ }
      else if (code == -2 || code == -1) { print "FAIL " cls "#" t; n++; bad++ }
      else if (code == -3) { print "IGNORED " cls "#" t }
      else if (code == -4) { print "SKIPPED " cls "#" t " (assumption)" }
    }
    /^INSTRUMENTATION_RESULT: shortMsg=/ { crash = $0 }
    /^INSTRUMENTATION_CODE: / { done = 1 }
    END {
      if (crash != "") { print "FAIL " cls " (instrumentation: " crash ")"; bad++ }
      if (!done) { print "FAIL " cls " (no result)"; bad++ }
      if (n == 0 && bad == 0) { print "FAIL " cls " (no tests ran)"; bad++ }
      exit (bad > 0 ? 1 : 0)
    }' "$f" | tee -a "$OUT/results.txt"
  if [ "${PIPESTATUS[0]}" != 0 ]; then
    ok=0
    # The failing test's message and stack, short.
    grep -A12 "^INSTRUMENTATION_STATUS: stack=" "$f" | head -40
    adb exec-out screencap -p > "$OUT/screens/after-$c.png" 2>/dev/null || true
  fi
done

adb pull "$DEV_DIR/." "$OUT/screens/" >/dev/null 2>&1 || true
adb pull "/sdcard/Android/data/$E2E/files/screens/." "$OUT/screens/" >/dev/null 2>&1 || true
adb logcat -d > "$OUT/logcat.txt" 2>&1 || true
# No crash and no ANR in Guard or the test app (design 6.2 step 7).
if grep -E "FATAL EXCEPTION|ANR in" -A3 "$OUT/logcat.txt" | grep -qE "Process: ($PKG|$E2E)[ ,]|ANR in ($PKG|$E2E)"; then
  echo "e2e: a crash or ANR in logcat:"; grep -E "FATAL EXCEPTION|ANR in" -A8 "$OUT/logcat.txt" | head -40
  ok=0
fi

echo "== results (API $API)"; cat "$OUT/results.txt"
if [ "$ok" = 1 ]; then date -u +%FT%TZ > "$OUT/passed"; echo "e2e: all passed"; exit 0; fi
echo "e2e: FAILED"; exit 1
