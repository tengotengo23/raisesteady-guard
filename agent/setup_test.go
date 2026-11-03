package main

import (
	"bufio"
	"bytes"
	"errors"
	"strings"
	"testing"

	"raisesteady-guard/agent/internal/agent"
	"raisesteady-guard/agent/internal/api"
	"raisesteady-guard/agent/internal/core"
	"raisesteady-guard/agent/internal/detect"
	"raisesteady-guard/agent/internal/guard"
	"raisesteady-guard/agent/internal/policy"
)

type setupSys struct{ reg *policy.Fake }

func (s *setupSys) Reg() policy.Registry { return s.reg }
func (s *setupSys) MountHive(string) (string, func() error, error) {
	return "", nil, errors.New("no")
}
func (s *setupSys) SignedIn(string) bool { return false }
func (s *setupSys) LocalAccounts() ([]api.LocalAccount, error) {
	return []api.LocalAccount{{SID: "1000", Name: "mum", Admin: true}, {SID: "1001", Name: "kid"}, {SID: "1002", Name: "dad", Admin: true}}, nil
}
func (s *setupSys) UserLang(string) string   { return "" }
func (s *setupSys) SystemLang() string       { return "en" }
func (s *setupSys) DetectEnv() detect.Env    { return nil }
func (s *setupSys) GuardSys() guard.System   { return nil }
func (s *setupSys) SecureDir(string) error   { return nil }
func (s *setupSys) IsPerson(sid string) bool { return sid != "0" }
func (s *setupSys) DetectBrowsers(core.UnsupportedList) []detect.Found {
	return []detect.Found{{ID: "chrome", Name: "Google Chrome", Supported: true}, {ID: "firefox", Name: "Firefox", Supported: true}, {ID: "opera", Name: "Opera"}}
}

type setupSvc struct{ installed, started bool }

func (s *setupSvc) Install(string) error { s.installed = true; return nil }
func (s *setupSvc) Start() error         { s.started = true; return nil }
func (s *setupSvc) Stop() error          { return nil }
func (s *setupSvc) Delete() error        { return nil }
func (s *setupSvc) Running() bool        { return s.started }

func setupRun(t *testing.T, paths agent.Paths, input string, term bool, f setupFlags) (int, string, *setupSvc) {
	t.Helper()
	data, err := core.LoadData()
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	in := bufio.NewReader(strings.NewReader(input))
	sio := &setupIO{
		in: in, out: &out, isTerm: term, lang: "en", exePath: "/usr/lib/raisesteady-guard/raisesteady-guard",
		secret: func() (string, error) { return readSecret(in) },
		getenv: func(k string) string { return map[string]string{"SUDO_USER": "mum"}[k] },
	}
	svc := &setupSvc{}
	code := runSetup(sio, f, &setupSys{reg: policy.NewFake()}, svc, data, paths)
	return code, out.String(), svc
}

func TestSetupInteractive(t *testing.T) {
	paths := agent.PathsIn(t.TempDir())
	// Terms yes; age 3 (9 to 12); a bad PIN, then 1234 twice; no family filter.
	code, out, svc := setupRun(t, paths, "yes\n3\n12\n12\n1234\n1234\nn\n", true, setupFlags{})
	if code != exitOK {
		t.Fatalf("code %d\n%s", code, out)
	}
	for _, want := range []string{"made with AI", "Support is voluntary", "every account on this computer", "dad", "Google Chrome, Firefox", "closed whenever someone opens them on this computer: Opera", "The PIN must be 4 to 8 digits", "Done."} {
		if !strings.Contains(out, want) {
			t.Errorf("output lacks %q", want)
		}
	}
	if strings.Contains(out, "mum,") || strings.Contains(out, ": mum") {
		t.Error("the parent running setup is listed as another administrator")
	}
	if !svc.installed || !svc.started {
		t.Error("service not installed and started")
	}
	var cfg core.Config
	if err := core.LoadJSON(paths.Config, &cfg); err != nil {
		t.Fatal(err)
	}
	if cfg.Mode != core.ModeAll || len(cfg.Accounts) != 1 || cfg.Accounts[0].Band != "9-12" || !cfg.Accounts[0].Settings.FamilyDNSOff {
		t.Errorf("config = mode %s, accounts %+v", cfg.Mode, cfg.Accounts)
	}
	if !cfg.Pin.Check("1234") {
		t.Error("PIN not saved")
	}
	// A second run says it is set up and changes nothing.
	code, out, svc = setupRun(t, paths, "", true, setupFlags{})
	if code != exitOK || !strings.Contains(out, "already set up") || svc.installed {
		t.Errorf("second run: code %d, installed %v\n%s", code, svc.installed, out)
	}
}

func TestSetupRefusedTermsChangeNothing(t *testing.T) {
	paths := agent.PathsIn(t.TempDir())
	code, out, svc := setupRun(t, paths, "no\n", true, setupFlags{})
	if code == exitOK || !strings.Contains(out, "Nothing was changed") || svc.installed {
		t.Errorf("code %d installed %v\n%s", code, svc.installed, out)
	}
	if core.LoadJSON(paths.Config, &core.Config{}) == nil {
		t.Error("config written after refusing the terms")
	}
}

func TestSetupNeedsATerminalOrFlags(t *testing.T) {
	paths := agent.PathsIn(t.TempDir())
	if code, _, _ := setupRun(t, paths, "", false, setupFlags{}); code != exitUsage {
		t.Errorf("no terminal, no --yes: code %d", code)
	}
	code, out, svc := setupRun(t, paths, "", false, setupFlags{yes: true, band: "13-15", pin: "56789", familyDNS: "on"})
	if code != exitOK || !svc.started {
		t.Fatalf("--yes: code %d\n%s", code, out)
	}
	if strings.Contains(out, "made with AI") {
		t.Error("--yes printed the terms")
	}
}

func TestSetupStringsComplete(t *testing.T) {
	for key, m := range setupText {
		for _, l := range []string{"en", "ka", "ru"} {
			if strings.TrimSpace(m[l]) == "" {
				t.Errorf("%s has no %s text", key, l)
			}
			if strings.Count(m[l], "%") != strings.Count(m["en"], "%") {
				t.Errorf("%s/%s: placeholders differ from English", key, l)
			}
		}
	}
	data, _ := core.LoadData()
	for _, b := range data.Bands.List {
		if _, ok := bandLabel[b.ID]; !ok {
			t.Errorf("no label for band %s", b.ID)
		}
	}
	for _, l := range []string{"en", "ka", "ru"} {
		if !strings.Contains(core.Terms(l), "GPL-3.0") {
			t.Errorf("terms %s not embedded", l)
		}
	}
}
