package agent

import (
	"encoding/json"
	"errors"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"raisesteady-guard/agent/internal/api"
	"raisesteady-guard/agent/internal/core"
	"raisesteady-guard/agent/internal/detect"
	"raisesteady-guard/agent/internal/guard"
	"raisesteady-guard/agent/internal/policy"
	"raisesteady-guard/agent/internal/store"
)

const (
	parentSID = "S-1-5-21-7-8-9-1000"
	kidOn     = "S-1-5-21-7-8-9-1001" // signed in: hive loaded
	kidOff    = "S-1-5-21-7-8-9-1002" // signed out, has a profile
	kidNew    = "S-1-5-21-7-8-9-1003" // never signed in
)

type fakeSys struct {
	reg      *policy.Fake
	profiles map[string]bool
	mounts   int
	secured  []string
}

func newFakeSys() *fakeSys {
	return &fakeSys{reg: policy.NewFake(`HKU\`+kidOn, `HKU\`+parentSID), profiles: map[string]bool{kidOff: true}}
}

func (f *fakeSys) Reg() policy.Registry { return f.reg }
func (f *fakeSys) MountHive(sid string) (string, func() error, error) {
	if !f.profiles[sid] {
		return "", nil, core.ErrNoProfile
	}
	root := `HKU\KG_` + sid
	f.reg.Roots[root] = true
	f.mounts++
	return root, func() error { delete(f.reg.Roots, root); return nil }, nil
}
func (f *fakeSys) SignedIn(sid string) bool { return f.reg.RootAvailable(`HKU\` + sid) }
func (f *fakeSys) LocalAccounts() ([]api.LocalAccount, error) {
	return []api.LocalAccount{{SID: parentSID, Name: "Parent", Admin: true}, {SID: kidOn, Name: "Ana"}, {SID: kidOff, Name: "Luka"}, {SID: kidNew, Name: "Nino"}}, nil
}
func (f *fakeSys) UserLang(sid string) string {
	if f.SignedIn(sid) {
		return "ka"
	}
	return ""
}
func (f *fakeSys) SystemLang() string       { return "en" }
func (f *fakeSys) DetectEnv() detect.Env    { return env{f.reg} }
func (f *fakeSys) GuardSys() guard.System   { return nil }
func (f *fakeSys) SecureDir(p string) error { f.secured = append(f.secured, p); return nil }

type env struct{ reg *policy.Fake }

func (e env) Reg() policy.Registry      { return e.reg }
func (env) ProgramDirs() []string       { return []string{`C:\Program Files`} }
func (env) Profiles() map[string]string { return map[string]string{} }
func (env) Exists(p string) bool {
	return strings.HasSuffix(p, `chrome.exe`) || strings.HasSuffix(p, `opera.exe`)
}
func (env) ProductName(p string) string {
	if strings.HasSuffix(p, "opera.exe") {
		return "Opera Internet Browser"
	}
	return "Google Chrome"
}

type fakeSvc struct {
	installed, running bool
	exe                string
	stops, starts      int
}

func (s *fakeSvc) Install(exe string) error { s.installed, s.exe = true, exe; return nil }
func (s *fakeSvc) Start() error {
	if !s.installed {
		return errors.New("not installed")
	}
	s.running = true
	s.starts++
	return nil
}
func (s *fakeSvc) Stop() error   { s.running = false; s.stops++; return nil }
func (s *fakeSvc) Delete() error { s.installed = false; return nil }
func (s *fakeSvc) Running() bool { return s.running }

func setup(t *testing.T) (*fakeSys, *fakeSvc, *core.Data, Paths) {
	t.Helper()
	d, err := core.LoadData()
	if err != nil {
		t.Fatal(err)
	}
	return newFakeSys(), &fakeSvc{}, d, PathsIn(filepath.Join(t.TempDir(), "RaiseSteadyGuard"))
}

const chrome = `Software\Policies\Google\Chrome`

func TestInstallPerUserAndUninstall(t *testing.T) {
	sys, svc, d, p := setup(t)
	// Values that are there before us must survive the uninstall.
	_ = sys.reg.Set(`HKU\`+kidOn, chrome, "HomepageLocation", policy.Str("https://school.example/"))
	_ = sys.reg.Set(`HKU\`+kidOn, chrome, "ForceGoogleSafeSearch", policy.DW(0))
	_ = sys.reg.Set("HKLM", `Software\Policies\Microsoft\Edge`, "SomethingElse", policy.DW(1))
	before := sys.reg.Dump()

	res, err := Install(sys, svc, d, p, InstallOptions{Accounts: kidOn + "," + kidOff + "," + kidNew, Band: "9-12", Pin: "2468", ExePath: `C:\Program Files\RaiseSteadyGuard\raisesteady-guard-agent.exe`}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !svc.installed || !svc.running || svc.exe == "" {
		t.Fatalf("service: %+v", svc)
	}
	if len(sys.secured) != 1 || sys.secured[0] != p.Dir {
		t.Fatalf("data folder not secured: %v", sys.secured)
	}
	if sys.mounts != 1 || sys.reg.Roots[`HKU\KG_`+kidOff] {
		t.Fatalf("the signed-out hive must be loaded once and unloaded: mounts=%d", sys.mounts)
	}
	n := len(res.Errors)
	if n != 0 || res.Written == 0 || res.Pending == 0 {
		t.Fatalf("install result: %+v", res)
	}
	var cfg core.Config
	if err := core.LoadJSON(p.Config, &cfg); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(p.Config)
	if strings.Contains(string(raw), `"2468"`) || !cfg.Pin.Check("2468") {
		t.Fatal("PIN must be stored only as a hash")
	}
	toks := map[string]bool{}
	for _, a := range cfg.Accounts {
		toks[a.Token] = true
	}
	if len(cfg.Accounts) != 3 || len(toks) != 3 || cfg.Accounts[1].Name != "Luka" {
		t.Fatalf("accounts: %+v", cfg.Accounts)
	}
	tokOff := cfg.Accounts[1].Token
	// Policies: in the signed-in hive and in the mounted hive, none in HKLM, none for the parent.
	mk := chrome + `\3rdparty\extensions\` + d.ExtIDs.ChromiumDevID + `\policy`
	if v, _ := sys.reg.Get(`HKU\`+kidOn, mk, "agentToken"); v.S != cfg.Accounts[0].Token {
		t.Error("signed-in kid: managed token missing")
	}
	if v, _ := sys.reg.Get(`HKU\KG_`+kidOff, mk, "agentToken"); v.S != tokOff {
		t.Error("signed-out kid: managed token missing in the loaded hive")
	}
	if v, _ := sys.reg.Get(`HKU\`+kidOn, chrome, "ForceGoogleSafeSearch"); v.D != 1 {
		t.Error("safe search not forced")
	}
	for k := range sys.reg.Dump() {
		if strings.HasPrefix(k, `HKLM\`) && !strings.Contains(k, "SomethingElse") || strings.HasPrefix(k, `HKU\`+parentSID) {
			t.Errorf("wrote where it must not: %s", k)
		}
	}

	// The parent customizes Ana's settings and the time zone.
	cfg.Accounts[0].BrowseMin, cfg.Accounts[0].Allow, cfg.Accounts[0].FocusMode = 33, []string{"school.example"}, false
	cfg.UTCOffsetMin = 240
	if err := core.SaveJSON(p.Config, cfg); err != nil {
		t.Fatal(err)
	}
	// A reinstall needs the PIN, keeps every kept account's settings and token,
	// drops Nino, and gives a new account the chosen band.
	if _, err := Install(sys, svc, d, p, InstallOptions{Accounts: kidOn + "," + kidOff, Band: "13-15", Pin: "1111", ExePath: "x"}, nil); !errors.Is(err, ErrWrongPin) {
		t.Fatalf("reinstall with a wrong PIN: %v", err)
	}
	if _, err := Install(sys, svc, d, p, InstallOptions{Accounts: kidOn + "," + parentSID, Band: "13-15", Pin: "2468", ExePath: "x"}, nil); err != nil {
		t.Fatal(err)
	}
	var cfg2 core.Config
	_ = core.LoadJSON(p.Config, &cfg2)
	a0 := cfg2.Accounts[0]
	if a0.Token != cfg.Accounts[0].Token || a0.Band != "9-12" || a0.BrowseMin != 33 || len(a0.Allow) != 1 || a0.FocusMode || cfg2.UTCOffsetMin != 240 {
		t.Fatalf("reinstall must keep Ana's settings: %+v offset %d", a0, cfg2.UTCOffsetMin)
	}
	if len(cfg2.Accounts) != 2 || cfg2.Accounts[1].Band != "13-15" || cfg2.Accounts[1].Token == tokOff {
		t.Fatalf("new account: %+v", cfg2.Accounts)
	}
	if v, _ := sys.reg.Get(`HKU\`+kidOn, chrome, "ForceYouTubeRestrict"); v.D != 2 {
		t.Error("Ana stays 9-12 (YouTube strict)")
	}
	if v, _ := sys.reg.Get(`HKU\`+parentSID, chrome, "ForceYouTubeRestrict"); v.D != 1 {
		t.Error("the new account gets 13-15 (YouTube moderate)")
	}
	_ = tokOff

	// Uninstall: a wrong PIN changes nothing.
	if _, err := Uninstall(sys, svc, d, p, "0000", nil); !errors.Is(err, ErrWrongPin) {
		t.Fatalf("uninstall wrong PIN: %v", err)
	}
	if !svc.installed || !fileExists(p.Config) {
		t.Fatal("a wrong PIN must not remove anything")
	}
	res, err = Uninstall(sys, svc, d, p, "2468", nil)
	if err != nil {
		t.Fatal(err)
	}
	if svc.installed || svc.running || fileExists(p.Dir) {
		t.Fatalf("left behind: service=%v dir=%v", svc.installed, fileExists(p.Dir))
	}
	after := sys.reg.Dump()
	for k, v := range before {
		if got, ok := after[k]; !ok || !got.Equal(v) {
			t.Errorf("%s: was %+v, now %+v", k, v, got)
		}
	}
	for k := range after {
		if _, ok := before[k]; !ok {
			t.Errorf("policy left behind: %s", k)
		}
	}
}

func fileExists(p string) bool { _, err := os.Stat(p); return err == nil }

// --family-dns off starts new accounts without the AdGuard resolver, and no DoH value is written.
func TestInstallFamilyDNSOff(t *testing.T) {
	sys, svc, d, p := setup(t)
	if _, err := Install(sys, svc, d, p, InstallOptions{Accounts: kidOn, Band: "9-12", Pin: "2468", ExePath: `C:\Program Files\RaiseSteadyGuard\raisesteady-guard-agent.exe`, FamilyDNSOff: true}, nil); err != nil {
		t.Fatal(err)
	}
	var cfg core.Config
	if err := core.LoadJSON(p.Config, &cfg); err != nil {
		t.Fatal(err)
	}
	if len(cfg.Accounts) != 1 || !cfg.Accounts[0].FamilyDNSOff {
		t.Fatalf("account should start with the family DNS off: %+v", cfg.Accounts)
	}
	if _, err := sys.reg.Get(`HKU\`+kidOn, chrome, "DnsOverHttpsTemplates"); err == nil {
		t.Fatal("DoH template written although the family DNS is off")
	}
	if v, err := sys.reg.Get(`HKU\`+kidOn, chrome, "ForceGoogleSafeSearch"); err != nil || !v.Equal(policy.DW(1)) {
		t.Fatalf("other policies must still be written: %v %v", v, err)
	}
}

func TestInstallModeAllAndErrors(t *testing.T) {
	sys, svc, d, p := setup(t)
	if _, err := Install(sys, svc, d, p, InstallOptions{Accounts: "S-1-5-21-9-9-9-4444", Band: "9-12", Pin: "2468"}, nil); !errors.Is(err, ErrUsage) {
		t.Fatalf("unknown SID: %v", err)
	}
	if _, err := Install(sys, svc, d, p, InstallOptions{Accounts: "all", Band: "8-10", Pin: "2468"}, nil); !errors.Is(err, ErrUsage) {
		t.Fatalf("unknown band: %v", err)
	}
	if _, err := Install(sys, svc, d, p, InstallOptions{Accounts: "all", Band: "9-12", Pin: "12"}, nil); !errors.Is(err, ErrUsage) {
		t.Fatalf("short PIN: %v", err)
	}
	if _, err := Install(sys, svc, d, p, InstallOptions{Accounts: "x;y", Band: "9-12", Pin: "2468"}, nil); !errors.Is(err, ErrUsage) {
		t.Fatalf("bad accounts flag: %v", err)
	}
	if fileExists(p.Config) {
		t.Fatal("a failed install must not write a config")
	}
	if _, err := Install(sys, svc, d, p, InstallOptions{Accounts: "all", Band: "9-12", Pin: "2468"}, nil); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{chrome, `Software\Policies\Microsoft\Edge`, `Software\Policies\Mozilla\Firefox`} {
		if !sys.reg.KeyExists("HKLM", k) {
			t.Errorf("mode all: %s missing in HKLM", k)
		}
	}
	for k := range sys.reg.Dump() {
		if strings.HasPrefix(k, `HKU\`) {
			t.Fatalf("mode all wrote to a user hive: %s", k)
		}
	}
	a, err := Load(sys, d, p, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !a.Protected(parentSID) || a.Protected("S-1-5-18") {
		t.Fatal("mode all protects every person's account, not SYSTEM")
	}
	if _, err := Uninstall(sys, svc, d, p, "2468", nil); err != nil {
		t.Fatal(err)
	}
	if n := len(sys.reg.Dump()); n != 0 {
		t.Fatalf("%d values left", n)
	}
}

func TestLoopApplyDoesNotMountAndStatus(t *testing.T) {
	sys, svc, d, p := setup(t)
	if _, err := Install(sys, svc, d, p, InstallOptions{Accounts: kidOn + "," + kidNew, Band: "6-8", Pin: "2468"}, nil); err != nil {
		t.Fatal(err)
	}
	a, err := Load(sys, d, p, nil)
	if err != nil {
		t.Fatal(err)
	}
	m := sys.mounts
	r := a.Apply(false)
	if sys.mounts != m || r.Written != 0 || r.Pending == 0 {
		t.Fatalf("loop apply: mounts %d->%d, %+v", m, sys.mounts, r)
	}
	a.Detect()
	states := func() map[string]string {
		m := map[string]string{}
		for _, b := range a.BrowserStatus() {
			m[b.Name+"|"+b.Account] = b.State
			if b.State == "protected" {
				t.Fatal("never say protected")
			}
		}
		return m
	}
	st := states()
	if st["Google Chrome|Ana"] != "set" || st["Google Chrome|Nino"] != "waiting" || st["Opera|"] != "closed" {
		t.Fatalf("browser states (no extension yet, Nino not signed in): %v", st)
	}
	// Nino signs in: the next apply writes her policies too.
	sys.reg.Roots[`HKU\`+kidNew] = true
	if r := a.Apply(false); r.Pending != 0 || r.Written == 0 {
		t.Fatalf("after sign-in: %+v", r)
	}
	if st := states(); st["Google Chrome|Nino"] != "set" {
		t.Fatalf("Nino's Chrome: %v", st)
	}
	if !a.Protected(kidNew) || a.Protected(parentSID) {
		t.Fatal("Protected()")
	}
	// The parent page changes the band: applied at once.
	srv := a.Server()
	if srv.Port != core.DefaultPort {
		t.Fatal("port")
	}
	cfg := a.Config()
	b13, _ := d.Bands.Get("13-15")
	cfg.Accounts[0].Settings = core.SettingsForBand(b13)
	if err := a.SaveConfig(cfg); err != nil {
		t.Fatal(err)
	}
	if v, _ := sys.reg.Get(`HKU\`+kidOn, chrome, "ForceYouTubeRestrict"); v.D != 1 {
		t.Fatal("saved settings not applied at once")
	}
	if v, _ := sys.reg.Get(`HKU\`+kidNew, chrome, "ForceYouTubeRestrict"); v.D != 2 {
		t.Fatal("the other account must keep its band")
	}
	st2 := GetStatus(sys, svc, d, p, nil)
	if !st2.Installed || st2.Policies == 0 || len(st2.Accounts) != 2 {
		t.Fatalf("status: %+v", st2)
	}
}

func TestUninstallAfterProfileDeleted(t *testing.T) {
	sys, svc, d, p := setup(t)
	if _, err := Install(sys, svc, d, p, InstallOptions{Accounts: kidOff, Band: "6-8", Pin: "2468"}, nil); err != nil {
		t.Fatal(err)
	}
	delete(sys.profiles, kidOff) // the account and its profile were deleted
	if _, err := Uninstall(sys, svc, d, p, "2468", nil); err != nil {
		t.Fatalf("uninstall must finish when a profile is gone: %v", err)
	}
	if fileExists(p.Dir) || svc.installed {
		t.Fatal("not removed")
	}
}

func TestVerifyPinLockoutShared(t *testing.T) {
	sys, svc, d, p := setup(t)
	now := time.Date(2026, 11, 3, 12, 0, 0, 0, time.UTC)
	clock := func() time.Time { return now }
	if _, err := Install(sys, svc, d, p, InstallOptions{Accounts: "all", Band: "9-12", Pin: "2468"}, clock); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 4; i++ {
		if err := VerifyPin(p, "1111", clock); !errors.Is(err, ErrWrongPin) {
			t.Fatalf("try %d: %v", i, err)
		}
	}
	if err := VerifyPin(p, "1111", clock); !errors.Is(err, core.ErrLocked) {
		t.Fatalf("fifth: %v", err)
	}
	if _, err := Uninstall(sys, svc, d, p, "2468", clock); !errors.Is(err, core.ErrLocked) {
		t.Fatalf("uninstall while locked: %v", err)
	}
	now = now.Add(16 * time.Minute)
	if _, err := Uninstall(sys, svc, d, p, "2468", clock); err != nil {
		t.Fatal(err)
	}
}

func TestParseAccounts(t *testing.T) {
	m, s, err := ParseAccounts(" ALL ")
	if err != nil || m != core.ModeAll || s != nil {
		t.Fatal("all")
	}
	m, s, err = ParseAccounts(kidOn + ", " + kidOn + "," + kidOff)
	if err != nil || m != core.ModeUsers || len(s) != 2 {
		t.Fatalf("sids: %v %v", s, err)
	}
	for _, bad := range []string{"", ",", "S-1-5-21-1-2-3-1001 & calc", "Administrator"} {
		if _, _, err := ParseAccounts(bad); !errors.Is(err, ErrUsage) {
			t.Errorf("%q accepted", bad)
		}
	}
}

func TestBrowserActiveOnlyWithRecentReport(t *testing.T) {
	sys, svc, d, p := setup(t)
	base := time.Date(2026, 11, 3, 12, 0, 0, 0, time.UTC)
	now := base
	clock := func() time.Time { return now }
	if _, err := Install(sys, svc, d, p, InstallOptions{Accounts: kidOn, Band: "9-12", Pin: "2468"}, clock); err != nil {
		t.Fatal(err)
	}
	a, err := Load(sys, d, p, clock)
	if err != nil {
		t.Fatal(err)
	}
	a.Detect()
	chromeState := func() api.BrowserStatus {
		for _, b := range a.BrowserStatus() {
			if b.Name == "Google Chrome" {
				return b
			}
		}
		t.Fatal("no Chrome line")
		return api.BrowserStatus{}
	}
	if b := chromeState(); b.State != "set" || b.Account != "" {
		t.Fatalf("before any report: %+v", b)
	}
	a.Store.AddUsage(kidOn, store.Usage{Browser: "chrome", BrowseSec: 30})
	now = now.Add(4 * time.Minute)
	if b := chromeState(); b.State != "active" || b.AgoMin != 4 {
		t.Fatalf("after a report 4 minutes ago: %+v", b)
	}
	a.Store.AddUsage(kidOn, store.Usage{Browser: "edge", BrowseSec: 30})
	now = now.Add(16 * time.Minute)
	if b := chromeState(); b.State != "set" {
		t.Fatalf("a report older than 15 minutes is not active: %+v", b)
	}
}

func TestFixedOffsetClock(t *testing.T) {
	sys, svc, d, p := setup(t)
	// 19:30 UTC; the PC was at UTC+4 at install, so 23:30 there.
	now := time.Date(2026, 11, 3, 19, 30, 0, 0, time.UTC)
	clock := func() time.Time { return now }
	saved := time.Local
	time.Local = time.FixedZone("TEST+4", 4*3600)
	if _, err := Install(sys, svc, d, p, InstallOptions{Accounts: kidOn, Band: "9-12", Pin: "2468"}, func() time.Time { return now.In(time.Local) }); err != nil {
		t.Fatal(err)
	}
	// The child moves Windows to UTC-10 afterwards: the agent must not follow.
	time.Local = time.FixedZone("TEST-10", -10*3600)
	defer func() { time.Local = saved }()
	a, err := Load(sys, d, p, clock)
	if err != nil {
		t.Fatal(err)
	}
	if a.Config().UTCOffsetMin != 240 {
		t.Fatalf("offset recorded at install: %d", a.Config().UTCOffsetMin)
	}
	h := a.Server().Handler()
	tok := a.Config().Accounts[0].Token
	req := httptest.NewRequest("GET", "http://127.0.0.1:47615/v1/state", nil)
	req.Header.Set("X-KG-Token", tok)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	var st api.State
	if err := json.Unmarshal(w.Body.Bytes(), &st); err != nil {
		t.Fatal(err)
	}
	if st.Day != "2026-11-03" || !strings.HasSuffix(st.Now, "+04:00") || !strings.HasPrefix(st.Now, "2026-11-03T23:30") || !st.BedtimeActive {
		t.Fatalf("state follows the fixed offset: %+v", st)
	}
	// 20:30 UTC = 00:30 the next day at UTC+4: a new day, counted under its own key.
	a.Store.AddUsage(kidOn, store.Usage{Browser: "chrome", BrowseSec: 30})
	now = now.Add(time.Hour)
	a.Store.AddUsage(kidOn, store.Usage{Browser: "chrome", BrowseSec: 30})
	rep, _ := a.Store.Report(kidOn, 2)
	if rep[0].Day != "2026-11-03" || rep[1].Day != "2026-11-04" {
		t.Fatalf("usage days keyed by the fixed offset: %+v", rep)
	}
}
