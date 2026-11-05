# Developing RaiseSteady Guard

Technical notes for building, testing and changing the project. For what it does and how to install it, see the [README](../README.md). The folder is called `kidguard` inside the Nexodyne repository.

Commands run from the project folder (the repository root).

## Windows agent and installer

### Build (macOS, Linux, or Windows with Git Bash)

```
brew install go makensis                 # macOS, once (Linux: Go and the nsis package)
tools/build-agent.sh                     # vet (host + windows amd64/arm64), go test, both .exe
tools/build-installer.sh                 # also runs build-agent.sh; writes dist/RaiseSteadyGuardSetup.exe
tools/build-installer.sh --skip-agent    # reuse the built agents
```

- `KG_VERSION=0.3.1` sets the version (default 0.3.1). With `KG_WINRES=1` the agent build adds the Windows file details (product name, version, icon) through go-winres v0.3.3; the release build on GitHub sets it.
- `build-agent.sh` copies `shared/age-bands.json`, `bookmarks.json`, `ext-ids.json`, `unsupported-browsers.json`, `pc-apps.json` and `ext/ui/tokens.css` into the Go module for `go:embed` (git-ignored copies; never edit them there). Age bands are required; a missing optional file is built empty and the build says so (`raisesteady-guard-agent status` lists it under `missingData`).
- The installer ships `THIRD-PARTY-NOTICES.txt` and `agent/NOTICE-go.txt` (Go, x/sys, x/crypto, NSIS licences) into the install folder; the build stops with a message if the first is missing.
- Go module `agent/`, dependencies only `golang.org/x/sys` and `golang.org/x/crypto` (pinned in `go.mod`/`go.sum`).
- Put a Mozilla-signed `dist/raisesteady-guard.xpi` before building the installer and it ships next to the agent; Firefox then force-installs it from `file:///C:/Program%20Files/RaiseSteadyGuard/raisesteady-guard.xpi`. Release Firefox installs only signed add-ons. Another location: `install --firefox-xpi <url>` (for example an addons.mozilla.org download URL).
- Chrome, Edge and Brave force-install only store extensions on a home PC. Until `shared/ext-ids.json` has `chromeStoreId` / `edgeStoreId`, the policies name the development id; the stores do not know it, so the extension cannot install on a PC that is not joined to a domain. All other policies work without the store.

### Install options

Interactive: welcome page with the limits, the Windows accounts to protect (standard accounts ticked, administrators not, "everyone on this PC" only as an option), age and PIN (4 to 8 digits, twice), then the family DNS filter (on by default; the page says what AdGuard DNS sees). The Start menu gets "Parent settings" (`http://127.0.0.1:47615/parent`). Languages: English, Georgian, Russian.

Silent: `RaiseSteadyGuardSetup.exe /S /ACCOUNTS=all /BAND=9-12 /PIN=1234 [/DNS=off]` (or `/ACCOUNTS=S-1-5-21-...,S-1-5-21-...`). `/DNS=off` starts new accounts without the AdGuard resolver.

Uninstall: Settings, Apps, or `"C:\Program Files\RaiseSteadyGuard\uninstall.exe" /S /PIN=1234`. It needs an administrator and the PIN, removes every policy value the agent wrote (values that were there before are put back), the service, `C:\ProgramData\RaiseSteadyGuard`, the shortcut and the uninstall entry.

### What runs

- Service `RaiseSteadyGuard` (LocalSystem, automatic start, restarts after 5, 10, 30 s on failure; a standard user cannot stop it). Every 5 minutes, at start and when a protected user signs in: finds the browsers, writes the policies, only values that differ.
- Policies per protected account under `HKU\<SID>\Software\Policies\...` (mode "users"), or under `HKLM` (mode "all"). Every key the agent writes in a user's hive is owned by SYSTEM with SYSTEM and Administrators full control and the user read only, inheritance off. A signed-out child's hive is loaded for a moment only by `install`, `apply-once` and `uninstall`, never by the service loop. An account that has never signed in has no profile yet: its policies are written at its first sign-in.
- New tab videos and the background feed refresh follow the focus switch: with focus mode off the extension asks YouTube for nothing.
- Family DNS filter: per account (`familyDnsOff` in `config.json`, a switch on the parent page). When on, Chromium browsers get `DnsOverHttpsMode` secure with AdGuard DNS Family and Firefox the locked `DNSOverHTTPS` policy; when off, those values are removed and nothing else changes.
- Browsers protected: Chrome, Edge, Brave, Firefox. Vivaldi does not document its policy support, so it is treated like Opera, Tor Browser, Yandex and the rest of `shared/unsupported-browsers.json`: closed within 3 seconds in a protected account's session, with a short message in English, Georgian or Russian.
- Clock: the day, bedtime and `now` follow a fixed UTC offset recorded at install (`utcOffsetMin`, editable on the parent page), never the Windows time zone, which a standard user may change. Summer time is not followed.
- Parent page browser list: "Active" only when that browser's extension reported in the last 15 minutes; otherwise "Settings set · extension not active yet" (or "Settings follow at the child's next sign-in").
- Update or reinstall (needs the PIN): accounts that stay keep their settings, lists, switches and token; the installer ticks the accounts already protected; the age chosen applies only to new accounts.
- Process guard: every 3 seconds, matched by exe name, version-info product name or path (a Tor Browser `firefox.exe` is matched by its path).
- PC games and chat apps (0.3.0): `shared/pc-apps.json` lists them (Windows exe, product and path; Linux program name, Flatpak id, snap name; chat apps carry `minAge` and their `terms` URL). Per account `apps` keeps only the parent's changes from the defaults (games: game time; chat apps: blocked under `minAge`), `gameMin` the daily game time (absent: the band's video minutes). The guard loop adds the seconds since its last tick (at most 10) while a game-time app runs in the account, closes game-time apps when the game time (plus granted extra minutes) is used up, at bedtime, in homework time and while paused, and blocked apps always, with the usual message at most once a minute per app and session, and one warning 5 minutes before game time ends. Game minutes and closed apps go to `usage.json`.
- Weekend, homework time and pause (0.3.0): `weekend` (`on`, minutes, bedtime of Friday and Saturday nights), `homework` (`on`, `start` before `end`, Monday to Friday) and `pauseUntil` (RFC 3339) per account. `/v1/state` sends today's limits and bedtime, `paused`, `pausedUntil`, `homework`, `homeworkEnd`, `searches` and a `schedule` the extension uses when the agent has not answered for 3 minutes. Old config files load with all of these off.
- Topic filter (0.3.1): `shared/topics.json` holds the words (en, ka, ru; `[id, pattern, weight, unless]`), the topic sites and the helplines; `ext/content/topics.js` is the matcher (fixed rules only, no AI and no network: spelling tricks are undone first, then a search or a title with one clear word blocks, descriptions and headings need a score of 4, body text is never read). Per account `topicsOff` (absent: on); `/v1/state` sends `topics`. Blocked topic pages count under the reason `topic`. Tests: `tests/unit/e-topics.test.mjs`.
- Built-in videos (0.3.1): `shared/videos.json` is made by `YTDLP=/path/to/yt-dlp node tools/update-videos.mjs [--cache dir]` (yt-dlp, Unlicense, a pinned release binary outside the repository) from each curated channel's Videos tab, up to 12 per channel; the extension merges it with the channel feeds and uses it alone when they do not answer. Re-run it before each release.
- Files in `C:\ProgramData\RaiseSteadyGuard\` (SYSTEM and Administrators only): `config.json` (settings, argon2id PIN hash, one random 64-hex token per account), `state.json` (every value the agent wrote, for a clean uninstall), `usage.json` (30 days), `lockout.json`, `agent.log` (no PIN, no tokens).

### Commands (`raisesteady-guard-agent.exe`)

`service`, `install --accounts all|SID,SID --band ID (--pin PIN | --pin-env KG_PIN) [--family-dns on|off]`, `uninstall`, `verify-pin`, `apply-once`, `status` (JSON, no secrets), `list-accounts [--ini PATH]`, `version`. Exit codes: 0 ok, 1 error, 2 usage, 3 wrong PIN, 4 PIN locked (5 wrong tries, 15 minutes), 5 needs an administrator. The installer passes the PIN in the environment variable `KG_PIN`, not on a command line.

### Localhost API (127.0.0.1 only)

Endpoints: `/v1/state`, `/v1/usage`, `/v1/event`, `/v1/grant`, `/v1/pin`, `/v1/request` for the extension (JSON, header `X-KG-Token`), and `/parent` for the parent page. Code: `agent/internal/api/`.
- Every response: `X-Content-Type-Options: nosniff`, `Cache-Control: no-store`. Hosts other than `127.0.0.1:<port>` and `localhost:<port>` get 421 (DNS rebinding).
- `Origin` absent, `chrome-extension://<our id>` or `moz-extension://<uuid>`: allowed, the origin is echoed in `Access-Control-Allow-Origin` (never `*`). Any other origin, and any preflight from one: 403.
- Token missing or unknown: 401. More than 300 calls a minute per token: 429. POST bodies must be `application/json`, at most 16 KB.
- `/v1/usage`: each field capped to the seconds since that browser's last report, at most 120 (a browser's first report: 60).
- `/v1/pin`: right PIN `200 {"ok":true}`; wrong `200 {"ok":false,"error":"pin","triesLeft":n}`; locked `429 {"ok":false,"locked":true,"retryAfterSec":n}`. `/v1/grant` answers a wrong PIN with 403 and the same body. The lock is shared by the API, the parent page and the uninstaller.
- `/v1/grant` extra: 1 to 120 minutes, at most 240 a day; `resume` ends a pause. `/v1/event` reasons: adult, gambling, bypass, social, content, video, time, bedtime, homework, paused, parent (at most 5000 a day).
- `/v1/usage` `searches`: search words with a count (at most 20 a call, 50 different a day, 100 characters); dropped while the parent has searches off.
- `/v1/request` (no PIN): `{"kind":"site","domain":...,"note":...}` or `{"kind":"time","note":...}`; note at most 140 characters; up to 20 open per account, dropped after 7 days, the same request as the newest one is not added twice (`"dup":true`). The parent page answers it (`/parent/request`).
- PIN tries are counted per source (parent page, each extension token, the installer); the parent page shows today's wrong tries.
- Parent page `/parent`: PIN login, session cookie `HttpOnly; SameSite=Strict; Path=/parent` without Max-Age (10 minutes absolute), PIN inputs `autocomplete="off"`, a CSRF token in every form, same-origin checks (Origin and Sec-Fetch-Site), CSP `default-src 'none'`, no JavaScript. Saving applies the policies at once.

### Tests

- `go test ./...` in `agent/` (fake registry, fake processes, httptest): policy maps per band and browser, the family DNS switch, per-user versus HKLM, rewrite only on change, uninstall removes only ours, install and uninstall flows, API token, origin, preflight, caps, PIN lockout, parent-page CSRF and cookies.
- `.github/workflows/windows.yml` (in the Nexodyne repository: `kidguard-windows.yml`), on windows-latest: build, silent install for everyone, service and recovery, Chrome, Edge and Firefox policies and managed config, ACL of the data folder, API with the token, web origin refused, wrong PIN refused, uninstall, HKLM\SOFTWARE\Policies equal to before; then per-user mode with `/DNS=off` for a new standard user, owner and ACL of the keys, that user failing to change, create or delete policy keys, and Edge (and Chrome) started as that user showing the policies on edge://policy / chrome://policy (reported as not verified when the runner cannot start a browser as that user).

## Extension

One Manifest V3 codebase in `ext/` for Chrome, Edge, Brave and Firefox (140+); Vivaldi is closed as unsupported (see above).

Build (Node 20+):

```
node tools/build-ext.mjs          # dev and store builds, zips
node tools/build-ext.mjs --dev    # dev builds only
```

- `dist/chromium-dev/`, `dist/firefox-dev/`: dev builds. The Chromium one carries the development public key (stable id in `shared/ext-ids.json`) and turns on the test hook `kg:core:test`. Load unpacked for testing only. The matching private key is kept outside the repository.
- `dist/chromium/`, `dist/firefox/` and `dist/raisesteady-guard-<version>-{chromium,firefox}.zip`: store builds, without the key and without the test hook.
- The build copies `shared/*.json` into `ext/data/` (never edit `ext/data/` by hand), writes the display name from `ext/ui/brand.js` (`APP_NAME`) into `_locales`, makes the icons if missing, and checks every file the manifest names, the rulesets (valid JSON, unique ids, 12 MB in total at most) and the translation keys.
- Block lists: `tools/build-lists.mjs` rebuilds `ext/rules/*.json` from the open lists named in [SOURCES.md](../SOURCES.md); `tools/build-cosmetic.mjs` rebuilds `ext/rules/cosmetic.css` from EasyList.

Modes:

- Managed (the Windows agent wrote the browser policy): settings come from `storage.managed` (`ext/managed_schema.json`) and the agent at `http://127.0.0.1:<agentPort>` with `X-KG-Token`. Parent settings are on the agent's `/parent` page.
- Standalone (no policy, for example a Mac or a test machine): the options page sets age, PIN (PBKDF2-SHA256, 150 000 rounds), limits, bedtime and lists in `storage.local`. Without the Windows app a child who can open the extension settings can remove the extension; the options page says so.

Tests:

```
npm install                       # puppeteer, for the Chrome tests
node --test tests/unit/
node tools/build-ext.mjs --dev && node tests/e2e/a-e2e.mjs && node tests/e2e/b-e2e.mjs
node tests/e2e/c-managed.mjs      # managed mode against the Go test server (needs Go)
npx -y web-ext@10.8.0 lint -s dist/firefox
```

The Chrome tests map every host to a local server, so no real site is contacted (`b-e2e.mjs` also opens live YouTube when it can). `c-managed.mjs` starts `TestServeForE2E` (the real API and parent page on 127.0.0.1:47616, PIN 2468, a clock moved to a Wednesday), puts the dev build in managed mode through the dev-only test hook, and checks "Ask a parent", the paused page, the homework page, the searches in the report and the popup lines.

Permissions and why (for store review):

| Permission | Why |
|---|---|
| `storage` | settings, today's time, the offline queue for the agent |
| `declarativeNetRequest` | block listed sites, safe search, the YouTube Restricted Mode header, time and bedtime blocks |
| `webNavigation` | remember which site a tab tried to open, so the block page can name it and a parent can allow it |
| `alarms` | count time every 30 seconds, start a new day at midnight, check bedtime |
| `notifications` | the 5-minute warning, break and bedtime reminders |
| `idle` | stop counting browsing time when nobody uses the computer (60 s) |
| host `<all_urls>` | the page check on each page, the video timer, redirect rules and headers, and the local agent on 127.0.0.1 |

No `tabs`, `scripting`, `cookies` or `history` permission is used.

## Android app (`android/`)

Kotlin and Jetpack Compose, minSdk 29, targetSdk 36. The build needs only JDK 17 and the Android SDK (no Node): `android/build-logic` packs `shared/` and `ext/rules/` into the app's assets at build time.

```sh
cd android
./gradlew :app:testDebugUnitTest :app:lintDebug :app:assembleDebug   # JVM tests, lint (MissingTranslation is an error), debug APK
./gradlew :e2e:assembleDebug :e2e:assembleDebugAndroidTest           # the test app and its end-to-end tests
E2E_OUT=build/e2e bash tools/e2e.sh                                  # on a booted emulator or device (adb)
```

- End-to-end tests (`android/e2e`, another app so its traffic goes through the filter): SetupTest, FilterTest, GameTimeTest, GuardTest, ReportTest, ParentTest, LookTest. CI runs them on emulators with API 36 and 29 (`.github/workflows/android.yml`); LookTest saves screenshots of every screen at font scale 1.0 and 2.0, light and dark, in the `e2e-api*` artefacts (`screens/look/`).
- Debug builds only have test hooks (`app/src/debug`): settings and filter start, a clock offset, the child screens, a status and look log receiver. Release builds have none.
- Texts: `app/src/main/res/values*/strings*.xml` in English, Georgian and Russian; every string needs all three.
- RS emoji: `bash tools/emoji-to-vd.sh` converts the repo's `emoji/svg` files to `res/drawable/rs_*.xml` (svg2vectordrawable 2.9.1, pinned); commit the output.
- Version: `android/version.properties` (`versionName`; versionCode = major x 1,000,000 + minor x 1,000 + patch). Release tags do not set it.
- Signed release APK (owner's Mac): the key is `~/Developer/keys/guard-android/guard-release.p12` (PKCS12, RSA 4096, 30 years), its passwords in `passwords.env` next to it (mode 600), never in the repository. `bash tools/sign-release.sh` builds `raisesteady-guard-android-<version>.apk`, checks it with `apksigner verify` and prints the certificate SHA-256. Keep this key: Android updates and a later Play listing ("use my own key") need the same one.
- Releases: `.github/workflows/release.yml` (job `android`) signs the APK only when the repository secrets `ANDROID_KEYSTORE_B64`, `ANDROID_KEYSTORE_PASSWORD`, `ANDROID_KEY_ALIAS` and `ANDROID_KEY_PASSWORD` exist; otherwise it builds the APK for the record and the release notes say the Android app is not in that release.
