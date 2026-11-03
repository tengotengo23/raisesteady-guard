package agent

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"raisesteady-guard/agent/internal/api"
	"raisesteady-guard/agent/internal/core"
	"raisesteady-guard/agent/internal/policy"
)

// Errors with their own exit codes (see main.go).
var (
	ErrWrongPin = errors.New("the PIN is not right")
	ErrUsage    = errors.New("usage")
)

// InstallOptions are the install command's flags.
type InstallOptions struct {
	Accounts   string // "all" or "S-1-5-21-...,S-1-5-21-..."
	Band       string
	Pin        string
	Port       int
	ExePath    string
	FirefoxXPI string // install_url for Firefox; "" = default path
	// FamilyDNSOff starts new accounts without the AdGuard DNS Family resolver.
	// Accounts kept from an earlier install keep their own choice.
	FamilyDNSOff bool
}

// ParseAccounts turns the --accounts flag into a mode and SIDs.
func ParseAccounts(s string) (mode string, sids []string, err error) {
	s = strings.TrimSpace(s)
	if strings.EqualFold(s, "all") {
		return core.ModeAll, nil, nil
	}
	for _, p := range strings.Split(s, ",") {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if !core.ValidSID(p) {
			return "", nil, fmt.Errorf("%w: %q is not a SID", ErrUsage, p)
		}
		dup := false
		for _, x := range sids {
			dup = dup || strings.EqualFold(x, p)
		}
		if !dup {
			sids = append(sids, p)
		}
	}
	if len(sids) == 0 {
		return "", nil, fmt.Errorf("%w: --accounts needs all or SIDs", ErrUsage)
	}
	return core.ModeUsers, sids, nil
}

// VerifyPin checks a PIN against the installed configuration, with the shared lockout.
func VerifyPin(p Paths, pin string, now func() time.Time) error {
	var cfg core.Config
	if err := core.LoadJSON(p.Config, &cfg); err != nil {
		return fmt.Errorf("read config: %w", err)
	}
	l := &core.Lockout{Path: p.Lockout, Now: now}
	ok, _, _, err := l.Verify(cfg.Pin, pin)
	if err != nil {
		return err
	}
	if !ok {
		return ErrWrongPin
	}
	return nil
}

// Install writes the configuration, applies the policies once and starts the service.
func Install(sys System, svc Services, data *core.Data, p Paths, o InstallOptions, now func() time.Time) (policy.Result, error) {
	var none policy.Result
	if !core.ValidPin(o.Pin) {
		return none, fmt.Errorf("%w: the PIN must be 4 to 8 digits", ErrUsage)
	}
	band, ok := data.Bands.Get(o.Band)
	if !ok {
		return none, fmt.Errorf("%w: unknown age band %q", ErrUsage, o.Band)
	}
	mode, sids, err := ParseAccounts(o.Accounts)
	if err != nil {
		return none, err
	}
	if o.Port == 0 {
		o.Port = core.DefaultPort
	}
	var old core.Config
	haveOld := core.LoadJSON(p.Config, &old) == nil
	if haveOld {
		// Reinstall or update: only with the PIN already set.
		if err := VerifyPin(p, o.Pin, now); err != nil {
			return none, err
		}
	}
	names := map[string]string{}
	if mode == core.ModeUsers {
		local, err := sys.LocalAccounts()
		if err != nil {
			return none, fmt.Errorf("list accounts: %w", err)
		}
		for _, la := range local {
			names[strings.ToUpper(la.SID)] = la.Name
		}
		for _, s := range sids {
			if _, ok := names[strings.ToUpper(s)]; !ok {
				return none, fmt.Errorf("%w: %s is not an account on this PC", ErrUsage, s)
			}
		}
	}
	hash, err := core.HashPin(o.Pin)
	if err != nil {
		return none, err
	}
	clock := now
	if clock == nil {
		clock = time.Now
	}
	cfg := core.Config{
		Version: 1, InstallID: core.RandomHex(16), Port: o.Port, Mode: mode, Pin: hash,
		ExtIDs: data.ExtIDs, FirefoxInstallURL: o.FirefoxXPI, SettingsVersion: 1,
		UTCOffsetMin: core.SystemOffsetMin(clock()),
	}
	// On a reinstall or update every account that stays keeps its settings,
	// lists, switches and token; new accounts start from the chosen band.
	kept := map[string]core.Account{}
	if haveOld {
		if old.InstallID != "" {
			cfg.InstallID = old.InstallID
		}
		if core.ValidOffset(old.UTCOffsetMin) {
			cfg.UTCOffsetMin = old.UTCOffsetMin
		}
		if cfg.FirefoxInstallURL == "" {
			cfg.FirefoxInstallURL = old.FirefoxInstallURL
		}
		cfg.SettingsVersion = old.SettingsVersion + 1
		for _, a := range old.Accounts {
			if core.ValidToken(a.Token) {
				kept[strings.ToUpper(a.SID)] = a
			}
		}
	}
	st := core.SettingsForBand(band)
	st.FamilyDNSOff = o.FamilyDNSOff
	account := func(sid, name string) core.Account {
		if a, ok := kept[strings.ToUpper(sid)]; ok && a.Settings.Validate(&data.Bands) == nil {
			if name != "" {
				a.Name = name
			}
			return a
		}
		return core.Account{SID: sid, Name: name, Token: core.NewToken(), Settings: st}
	}
	if mode == core.ModeAll {
		cfg.Accounts = []core.Account{account(core.AllSID, "*")}
	} else {
		for _, s := range sids {
			cfg.Accounts = append(cfg.Accounts, account(s, names[strings.ToUpper(s)]))
		}
	}
	if err := cfg.Validate(&data.Bands); err != nil {
		return none, err
	}
	if err := os.MkdirAll(p.Dir, 0o700); err != nil {
		return none, err
	}
	if err := sys.SecureDir(p.Dir); err != nil {
		return none, fmt.Errorf("protect %s: %w", p.Dir, err)
	}
	if err := core.SaveJSON(p.Config, cfg); err != nil {
		return none, err
	}
	if haveOld {
		_ = svc.Stop()
	}
	a, err := Load(sys, data, p, now)
	if err != nil {
		return none, err
	}
	res := a.Apply(true)
	if err := svc.Install(o.ExePath); err != nil {
		return res, fmt.Errorf("install service: %w", err)
	}
	if err := svc.Start(); err != nil {
		return res, fmt.Errorf("start service: %w", err)
	}
	return res, nil
}

// Uninstall checks the PIN, stops the service, removes every policy the agent
// wrote, deletes the service and the data folder.
func Uninstall(sys System, svc Services, data *core.Data, p Paths, pin string, now func() time.Time) (policy.Result, error) {
	var none policy.Result
	if _, err := os.Stat(p.Config); err == nil {
		if err := VerifyPin(p, pin, now); err != nil {
			return none, err
		}
	}
	return removeAll(sys, svc, data, p, now)
}

// PackageRemove is Uninstall for a Linux package manager: only root can run
// it, and root is the parent's administrator account, so no PIN is asked.
func PackageRemove(sys System, svc Services, data *core.Data, p Paths, now func() time.Time) (policy.Result, error) {
	return removeAll(sys, svc, data, p, now)
}

func removeAll(sys System, svc Services, data *core.Data, p Paths, now func() time.Time) (policy.Result, error) {
	var none policy.Result
	if err := svc.Stop(); err != nil {
		return none, fmt.Errorf("stop service: %w", err)
	}
	var res policy.Result
	if _, err := os.Stat(p.State); err == nil {
		a := &Agent{Sys: sys, Data: data, Paths: p, Now: now}
		if err := core.LoadJSON(p.State, &a.st); err != nil {
			return none, fmt.Errorf("read state: %w", err)
		}
		if a.st.Langs == nil {
			a.st.Langs = map[string]string{}
		}
		res = a.Remove(true)
		if len(res.Errors) > 0 || len(a.st.Policy.Records) > 0 {
			// Keep the state so a second run can finish; restart protection.
			_ = svc.Start()
			return res, fmt.Errorf("could not remove %d policy value(s); nothing else was changed", len(a.st.Policy.Records))
		}
	}
	if err := svc.Delete(); err != nil {
		return res, fmt.Errorf("delete service: %w", err)
	}
	if err := os.RemoveAll(p.Dir); err != nil {
		return res, fmt.Errorf("delete %s: %w", p.Dir, err)
	}
	return res, nil
}

// Status is the status command's answer (no tokens, no PIN).
type Status struct {
	App             string              `json:"app"`
	Installed       bool                `json:"installed"`
	ServiceRunning  bool                `json:"serviceRunning"`
	Mode            string              `json:"mode,omitempty"`
	Port            int                 `json:"port,omitempty"`
	Accounts        []StatusAccount     `json:"accounts,omitempty"`
	ExtIDs          core.ExtIDs         `json:"extIds"`
	Policies        int                 `json:"policiesWritten"`
	LastApply       time.Time           `json:"lastApply,omitempty"`
	Browsers        []api.BrowserStatus `json:"browsers,omitempty"`
	MissingData     []string            `json:"missingData,omitempty"`
	SettingsVersion int                 `json:"settingsVersion,omitempty"`
}

// StatusAccount is one protected account in the status.
type StatusAccount struct {
	SID  string `json:"sid"`
	Name string `json:"name"`
	Band string `json:"band"`
}

// GetStatus builds the status from disk.
func GetStatus(sys System, svc Services, data *core.Data, p Paths, now func() time.Time) Status {
	s := Status{App: core.AppName, ServiceRunning: svc.Running(), ExtIDs: data.ExtIDs, MissingData: data.Missing}
	a, err := Load(sys, data, p, now)
	if err != nil {
		return s
	}
	cfg := a.Config()
	s.Installed, s.Mode, s.Port, s.SettingsVersion = true, cfg.Mode, cfg.Port, cfg.SettingsVersion
	for _, acct := range cfg.Accounts {
		s.Accounts = append(s.Accounts, StatusAccount{SID: acct.SID, Name: acct.Name, Band: acct.Band})
	}
	_, s.LastApply, s.Policies = a.LastResult()
	a.Detect()
	s.Browsers = a.BrowserStatus()
	return s
}
