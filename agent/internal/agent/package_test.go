package agent

import (
	"os"
	"strings"
	"testing"
	"time"

	"raisesteady-guard/agent/internal/core"
	"raisesteady-guard/agent/internal/policy"
)

// The install with no terminal (P2-194): the package's post-install step
// writes a configuration that waits for the parent's first-run setup.

const (
	storeID = "kjfgpfhdfdilbkibcegclnbnlfkokcke"
	devID   = "jihggkhiiiinnflomjjnpomgeijgfnlg"
	firefox = `Software\Policies\Mozilla\Firefox`
	amoURL  = "https://addons.mozilla.org/firefox/downloads/latest/raisesteady-guard/latest.xpi"
	xpiFile = "file:///usr/lib/raisesteady-guard/raisesteady-guard.xpi"
)

// storeSys is a system like Linux: the Firefox add-on comes from the store
// and Yandex Browser is protected too.
type storeSys struct{ *fakeSys }

func (storeSys) FirefoxFromStore() bool           { return true }
func (storeSys) PolicyBrowsers() []policy.Browser { return policy.LinuxBrowsers() }

func loadCfg(t *testing.T, p Paths) core.Config {
	t.Helper()
	var cfg core.Config
	if err := core.LoadJSON(p.Config, &cfg); err != nil {
		t.Fatal(err)
	}
	return cfg
}

func TestPendingSettingsRunNoLimit(t *testing.T) {
	_, _, d, _ := setup(t)
	cfg, err := PendingConfig(d, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.SetupPending || cfg.Mode != core.ModeAll || len(cfg.Accounts) != 1 || cfg.Accounts[0].SID != core.AllSID {
		t.Fatalf("pending config: %+v", cfg)
	}
	if cfg.Pin.Check("0000") || cfg.Pin.Check("") {
		t.Fatal("a waiting configuration must have no PIN")
	}
	if !core.ValidToken(cfg.Accounts[0].Token) || cfg.ExtIDs != d.ExtIDs {
		t.Fatalf("token or ids: %+v", cfg)
	}
	s := cfg.Accounts[0].Settings
	// The oldest band: the child's age is not known, so nothing is stricter
	// than it must be.
	for _, b := range d.Bands.List {
		if own, _ := d.Bands.Get(s.Band); b.MinAge > own.MinAge {
			t.Errorf("band %s is not the oldest (%s is older)", s.Band, b.ID)
		}
	}
	if s.FamilyDNSOff || !s.Adblock {
		t.Errorf("the protection must be on while waiting: %+v", s)
	}
	if got := s.GameMinutes(&d.Bands); got != 1440 {
		t.Errorf("game minutes = %d, want the whole day", got)
	}
	// No limit, bedtime or homework window at any minute of a week.
	start := time.Date(2026, 11, 9, 0, 0, 0, 0, time.UTC) // a Monday
	for m := 0; m < 7*24*60; m += 7 {
		at := start.Add(time.Duration(m) * time.Minute)
		if b, v := s.LimitsOn(at); b != 1440 || v != 1440 {
			t.Fatalf("%v: limits %d/%d", at, b, v)
		}
		if s.BedtimeActiveAt(at) || s.HomeworkActive(at) {
			t.Fatalf("%v: bedtime or homework runs before the setup", at)
		}
		if paused, _ := s.PausedAt(at); paused {
			t.Fatalf("%v: paused before the setup", at)
		}
	}
}

func TestPackageInstallProtectsAtOnce(t *testing.T) {
	fs, svc, d, p := setup(t)
	sys := storeSys{fs}
	before := fs.reg.Dump()

	res, fresh, err := PackageInstall(sys, svc, d, p, "/usr/lib/raisesteady-guard/raisesteady-guard", nil)
	if err != nil {
		t.Fatal(err)
	}
	if !fresh || len(res.Errors) != 0 || res.Written == 0 {
		t.Fatalf("fresh=%v result=%+v", fresh, res)
	}
	if !svc.installed || !svc.running || svc.exe != "/usr/lib/raisesteady-guard/raisesteady-guard" {
		t.Fatalf("the service must be installed and running: %+v", svc)
	}
	if len(fs.secured) != 1 || fs.secured[0] != p.Dir {
		t.Fatalf("data folder not secured: %v", fs.secured)
	}
	cfg := loadCfg(t, p)
	if !cfg.SetupPending || cfg.FirefoxInstallURL != "" {
		t.Fatalf("config: %+v", cfg)
	}
	// The browser protection is there before any setup: every Chromium
	// browser force-installs the store's extension and gets its settings.
	for _, k := range []string{chrome, `Software\Policies\Microsoft\Edge`, `Software\Policies\BraveSoftware\Brave`, policy.YandexKey} {
		if v, _ := fs.reg.Get("HKLM", k+`\ExtensionInstallForcelist`, "1"); v.S != storeID+";"+core.ChromeUpdateURL {
			t.Errorf("%s forcelist = %q", k, v.S)
		}
		if v, _ := fs.reg.Get("HKLM", k+`\3rdparty\extensions\`+storeID+`\policy`, "agentToken"); v.S != cfg.Accounts[0].Token {
			t.Errorf("%s: no managed token under the store id", k)
		}
	}
	if v, _ := fs.reg.Get("HKLM", chrome, "ForceGoogleSafeSearch"); v.D != 1 {
		t.Error("safe search not forced")
	}
	if v, _ := fs.reg.Get("HKLM", chrome, "IncognitoModeAvailability"); v.D != 1 {
		t.Error("private windows not switched off")
	}
	if v, _ := fs.reg.Get("HKLM", chrome, "DnsOverHttpsMode"); v.S != "secure" {
		t.Error("family DNS not on")
	}
	if v, _ := fs.reg.Get("HKLM", firefox, "ExtensionSettings"); !strings.Contains(v.S, amoURL) || strings.Contains(v.S, "file:") {
		t.Errorf("Firefox ExtensionSettings = %s", v.S)
	}
	st := GetStatus(sys, svc, d, p, nil)
	if !st.Installed || !st.SetupPending || !st.ServiceRunning || st.Policies == 0 {
		t.Fatalf("status: %+v", st)
	}

	// A second run (the package is installed again) is an update: the same
	// configuration, the same token, the service running again.
	_, fresh, err = PackageInstall(sys, svc, d, p, "/usr/lib/raisesteady-guard/raisesteady-guard", nil)
	if err != nil || fresh {
		t.Fatalf("second run: fresh=%v err=%v", fresh, err)
	}
	if again := loadCfg(t, p); again.InstallID != cfg.InstallID || again.Accounts[0].Token != cfg.Accounts[0].Token || !again.SetupPending {
		t.Fatalf("the second run changed the configuration: %+v", again)
	}
	if !svc.running || svc.stops == 0 {
		t.Fatalf("service after the second run: %+v", svc)
	}

	// Removing a package that was never set up needs no PIN and leaves nothing.
	if _, err := Uninstall(sys, svc, d, p, "", nil); err != nil {
		t.Fatal(err)
	}
	if svc.installed || svc.running || fileExists(p.Dir) {
		t.Fatalf("left behind: service=%v dir=%v", svc.installed, fileExists(p.Dir))
	}
	after := fs.reg.Dump()
	if len(after) != len(before) {
		for k := range after {
			if _, ok := before[k]; !ok {
				t.Errorf("policy left behind: %s", k)
			}
		}
	}
}

func TestSetupAfterThePackageNeedsNoOldPin(t *testing.T) {
	fs, svc, d, p := setup(t)
	sys := storeSys{fs}
	if _, _, err := PackageInstall(sys, svc, d, p, "exe", nil); err != nil {
		t.Fatal(err)
	}
	pending := loadCfg(t, p)

	if _, err := Install(sys, svc, d, p, InstallOptions{Accounts: "all", Band: "9-12", Pin: "2468", ExePath: "exe"}, nil); err != nil {
		t.Fatal(err)
	}
	cfg := loadCfg(t, p)
	if cfg.SetupPending || !cfg.Pin.Check("2468") {
		t.Fatalf("after setup: pending=%v", cfg.SetupPending)
	}
	a := cfg.Accounts[0]
	if cfg.InstallID != pending.InstallID || a.Token != pending.Accounts[0].Token {
		t.Error("the browsers already have the token: setup must keep it")
	}
	band, _ := d.Bands.Get("9-12")
	if a.Band != "9-12" || a.BrowseMin != band.BrowseMin || a.GameMin != nil || a.Bedtime.Start != band.Bedtime.Start {
		t.Errorf("the chosen age band's limits must apply after setup: %+v", a.Settings)
	}
	if st := GetStatus(sys, svc, d, p, nil); st.SetupPending {
		t.Error("status still says the setup waits")
	}
	// From here on it is a normal installation: PIN needed.
	if _, err := Uninstall(sys, svc, d, p, "", nil); err == nil {
		t.Fatal("uninstall without the PIN after setup")
	}
	if _, _, err := PackageInstall(sys, svc, d, p, "exe", nil); err != nil {
		t.Fatal(err)
	}
	if again := loadCfg(t, p); again.SetupPending || !again.Pin.Check("2468") || again.Accounts[0].Band != "9-12" {
		t.Fatalf("a package update changed the parent's setup: %+v", again)
	}
}

// An update from 0.4.0: the stored configuration names the development id
// and the bundled file. The new program's ids and the store address reach
// every browser, and nothing of the old id stays behind.
func TestPackageUpdateHealsOldIDs(t *testing.T) {
	fs, svc, d, p := setup(t)
	old := *d
	old.ExtIDs.ChromeStoreID = ""
	if _, err := Install(fs, svc, &old, p, InstallOptions{Accounts: "all", Band: "9-12", Pin: "2468", ExePath: "exe", FirefoxXPI: xpiFile}, nil); err != nil {
		t.Fatal(err)
	}
	if v, _ := fs.reg.Get("HKLM", chrome+`\ExtensionInstallForcelist`, "1"); !strings.HasPrefix(v.S, devID+";") {
		t.Fatalf("the old install should name the development id: %q", v.S)
	}
	if cfg := loadCfg(t, p); cfg.ExtIDs.ChromeStoreID != "" || cfg.FirefoxInstallURL != xpiFile {
		t.Fatalf("old config: %+v", cfg)
	}

	sys := storeSys{fs}
	_, fresh, err := PackageInstall(sys, svc, d, p, "exe", nil)
	if err != nil || fresh {
		t.Fatalf("update: fresh=%v err=%v", fresh, err)
	}
	for k, v := range fs.reg.Dump() {
		if strings.Contains(k, devID) || strings.Contains(v.S, devID) {
			t.Errorf("the development id is still in %s", k)
		}
		if strings.Contains(v.S, "file:") {
			t.Errorf("a file address is still in %s", k)
		}
	}
	if v, _ := fs.reg.Get("HKLM", chrome+`\ExtensionInstallForcelist`, "1"); v.S != storeID+";"+core.ChromeUpdateURL {
		t.Errorf("forcelist after the update = %q", v.S)
	}
	if v, _ := fs.reg.Get("HKLM", chrome+`\ExtensionInstallAllowlist`, "1"); v.S != storeID {
		t.Errorf("allowlist after the update = %q", v.S)
	}
	if v, _ := fs.reg.Get("HKLM", firefox, "ExtensionSettings"); !strings.Contains(v.S, amoURL) {
		t.Errorf("Firefox ExtensionSettings = %s", v.S)
	}
	a, err := Load(sys, d, p, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := a.Config(); got.ExtIDs != d.ExtIDs || got.FirefoxInstallURL != "" || !got.Pin.Check("2468") {
		t.Errorf("loaded config: ids %+v url %q", got.ExtIDs, got.FirefoxInstallURL)
	}
}

// A PC without internet: the person who installed asked for the bundled
// file (--firefox-xpi file:...). An update keeps that choice.
func TestChosenFileSurvivesAnUpdate(t *testing.T) {
	fs, svc, d, p := setup(t)
	sys := storeSys{fs}
	if _, err := Install(sys, svc, d, p, InstallOptions{Accounts: "all", Band: "9-12", Pin: "2468", ExePath: "exe", FirefoxXPI: xpiFile, FirefoxFromFile: true}, nil); err != nil {
		t.Fatal(err)
	}
	if _, _, err := PackageInstall(sys, svc, d, p, "exe", nil); err != nil {
		t.Fatal(err)
	}
	if v, _ := fs.reg.Get("HKLM", firefox, "ExtensionSettings"); !strings.Contains(v.S, xpiFile) {
		t.Errorf("Firefox ExtensionSettings = %s", v.S)
	}
	// Windows has no store default: its file address is never dropped.
	fs2, svc2, d2, p2 := setup(t)
	if _, err := Install(fs2, svc2, d2, p2, InstallOptions{Accounts: "all", Band: "9-12", Pin: "2468", ExePath: "exe", FirefoxXPI: "file:///C:/Program%20Files/RaiseSteadyGuard/raisesteady-guard.xpi"}, nil); err != nil {
		t.Fatal(err)
	}
	a, err := Load(fs2, d2, p2, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(a.Config().FirefoxInstallURL, "file:") {
		t.Errorf("Windows lost its file address: %q", a.Config().FirefoxInstallURL)
	}
	for _, e := range fs2.reg.Dump() {
		if strings.Contains(e.S, "YandexBrowser") {
			t.Error("Yandex policies on a system that did not ask for them")
		}
	}
}

func TestPackageInstallNeverReplacesAConfigItCannotRead(t *testing.T) {
	fs, svc, d, p := setup(t)
	if err := os.MkdirAll(p.Dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p.Config, []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, fresh, err := PackageInstall(storeSys{fs}, svc, d, p, "exe", nil)
	if err == nil || fresh {
		t.Fatalf("fresh=%v err=%v", fresh, err)
	}
	if b, _ := os.ReadFile(p.Config); string(b) != "{not json" {
		t.Errorf("the unreadable configuration was replaced: %q", b)
	}
	if svc.installed || len(fs.reg.Dump()) != 0 {
		t.Error("something was installed in spite of the error")
	}
}

// An update whose stored configuration cannot be used must leave the service
// that runs now alone: it keeps protecting until the fault is put right.
func TestPackageUpdateWithAnUnusableConfigKeepsTheServiceRunning(t *testing.T) {
	fs, svc, d, p := setup(t)
	if _, err := Install(fs, svc, d, p, InstallOptions{Accounts: "all", Band: "9-12", Pin: "2468", ExePath: "exe", FirefoxXPI: xpiFile}, nil); err != nil {
		t.Fatal(err)
	}
	if !svc.running {
		t.Fatal("the service should run after the install")
	}
	cfg := loadCfg(t, p)
	cfg.Port = 80 // a value this build refuses
	if err := core.SaveJSON(p.Config, cfg); err != nil {
		t.Fatal(err)
	}
	stops, starts := svc.stops, svc.starts

	_, fresh, err := PackageInstall(storeSys{fs}, svc, d, p, "exe", nil)
	if err == nil || fresh {
		t.Fatalf("fresh=%v err=%v", fresh, err)
	}
	if strings.Contains(err.Error(), "read config") {
		t.Fatalf("the configuration should be readable but refused: %v", err)
	}
	if !svc.running || svc.stops != stops || svc.starts != starts {
		t.Errorf("the running service was touched: running=%v stops %d to %d, starts %d to %d", svc.running, stops, svc.stops, starts, svc.starts)
	}
	if got := loadCfg(t, p); got.Port != 80 {
		t.Errorf("the stored configuration was changed: port %d", got.Port)
	}
}
