// Package agent ties the parts together: configuration, the 5-minute policy
// loop, the 3-second process guard, the localhost server, install and uninstall.
// Windows calls go through System, so everything here is tested with fakes.
package agent

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"raisesteady-guard/agent/internal/api"
	"raisesteady-guard/agent/internal/core"
	"raisesteady-guard/agent/internal/detect"
	"raisesteady-guard/agent/internal/guard"
	"raisesteady-guard/agent/internal/policy"
	"raisesteady-guard/agent/internal/store"
)

// System is everything the agent needs from Windows.
type System interface {
	Reg() policy.Registry
	// MountHive loads a signed-out user's NTUSER.DAT under HKU and returns its root.
	MountHive(sid string) (root string, unmount func() error, err error)
	SignedIn(sid string) bool
	LocalAccounts() ([]api.LocalAccount, error)
	UserLang(sid string) string // "" when unknown
	SystemLang() string
	DetectEnv() detect.Env
	GuardSys() guard.System
	SecureDir(path string) error // SYSTEM and Administrators only
}

// Services controls the Windows service.
type Services interface {
	Install(exePath string) error // create, automatic start, restart on failure
	Start() error
	Stop() error // waits until stopped; nil if not installed
	Delete() error
	Running() bool
}

// Paths are the agent's files.
type Paths struct {
	Dir, Config, State, Usage, Lockout, Log string
}

// PathsIn returns the standard file names in dir.
func PathsIn(dir string) Paths {
	return Paths{
		Dir: dir, Config: filepath.Join(dir, "config.json"), State: filepath.Join(dir, "state.json"),
		Usage: filepath.Join(dir, "usage.json"), Lockout: filepath.Join(dir, "lockout.json"), Log: filepath.Join(dir, "agent.log"),
	}
}

type stateFile struct {
	Policy    policy.State      `json:"policy"`
	Langs     map[string]string `json:"langs"`
	LastApply time.Time         `json:"lastApply"`
}

// Agent is the running agent.
type Agent struct {
	Sys   System
	Data  *core.Data
	Paths Paths
	Now   func() time.Time
	Log   func(format string, args ...any)

	Store *store.Store
	Lock  *core.Lockout
	Guard *guard.Guard

	mu      sync.Mutex // cfg
	cfg     core.Config
	applyMu sync.Mutex // st, last
	st      stateFile
	last    policy.Result
	foundMu sync.Mutex
	found   []detect.Found
}

func (a *Agent) now() time.Time {
	if a.Now != nil {
		return a.Now()
	}
	return time.Now()
}

// Local is the agent's clock: the time now at the fixed UTC offset in the
// configuration (never the Windows time zone, which a standard user may change).
func (a *Agent) Local() time.Time {
	return a.now().In(core.Zone(a.Config().UTCOffsetMin))
}

func (a *Agent) logf(format string, args ...any) {
	if a.Log != nil {
		a.Log(format, args...)
	}
}

// Load reads the configuration and state from disk.
func Load(sys System, data *core.Data, p Paths, now func() time.Time) (*Agent, error) {
	a := &Agent{Sys: sys, Data: data, Paths: p, Now: now}
	if err := core.LoadJSON(p.Config, &a.cfg); err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}
	if err := a.cfg.Validate(&data.Bands); err != nil {
		return nil, fmt.Errorf("config: %w", err)
	}
	if err := core.LoadJSON(p.State, &a.st); err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("read state: %w", err)
	}
	if a.st.Langs == nil {
		a.st.Langs = map[string]string{}
	}
	st, err := store.Open(p.Usage, a.Local)
	if err != nil {
		return nil, err
	}
	a.Store = st
	a.Lock = &core.Lockout{Path: p.Lockout, Now: a.Local}
	a.Guard = &guard.Guard{Sys: sys.GuardSys(), List: data.Unsupported, Protected: a.Protected, Now: now}
	if c, ok := sys.(interface {
		Classify(core.UnsupportedList, string, string) guard.Match
	}); ok {
		a.Guard.Classify = c.Classify // Linux
	}
	return a, nil
}

// Config returns a copy of the configuration.
func (a *Agent) Config() core.Config {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.cfg.Clone()
}

// SaveConfig validates, writes and applies a new configuration.
func (a *Agent) SaveConfig(c core.Config) error {
	if err := c.Validate(&a.Data.Bands); err != nil {
		return err
	}
	if err := core.SaveJSON(a.Paths.Config, c); err != nil {
		return err
	}
	a.mu.Lock()
	a.cfg = c.Clone()
	a.mu.Unlock()
	keep := map[string]bool{}
	for _, acct := range c.Accounts {
		keep[acct.SID] = true
	}
	a.Store.Prune(keep)
	r := a.Apply(false)
	a.logf("settings saved (version %d): %d written, %d removed, %d pending, %d errors", c.SettingsVersion, r.Written, r.Removed, r.Pending, len(r.Errors))
	return nil
}

// Protected reports whether a process owner's SID is protected.
func (a *Agent) Protected(sid string) bool {
	cfg := a.Config()
	if cfg.Mode == core.ModeAll {
		if p, ok := a.Sys.(interface{ IsPerson(sid string) bool }); ok {
			return p.IsPerson(sid) // Linux: a person's uid
		}
		// Every person's account: local, domain and Microsoft Entra users.
		return strings.HasPrefix(sid, "S-1-5-21-") || strings.HasPrefix(sid, "S-1-12-1-")
	}
	for _, acct := range cfg.Accounts {
		if strings.EqualFold(acct.SID, sid) {
			return true
		}
	}
	return false
}

func (a *Agent) lang(sid string) string {
	if l := a.Sys.UserLang(sid); l != "" {
		a.st.Langs[sid] = l
		return l
	}
	if l := a.st.Langs[sid]; l != "" {
		return l
	}
	return "en"
}

// targets lists where to write; it must run under applyMu.
func (a *Agent) targets(cfg core.Config) []policy.Target {
	if cfg.Mode == core.ModeAll {
		l := a.Sys.SystemLang()
		if l == "" {
			l = "en"
		}
		return []policy.Target{{Root: "HKLM", Account: cfg.Accounts[0], Lang: l}}
	}
	var out []policy.Target
	for _, acct := range cfg.Accounts {
		out = append(out, policy.Target{Root: policy.RootForSID(acct.SID), Account: acct, Lang: a.lang(acct.SID)})
	}
	return out
}

// Desired returns the entries for the current configuration (for status and tests).
func (a *Agent) Desired() []policy.Entry {
	a.applyMu.Lock()
	defer a.applyMu.Unlock()
	cfg := a.Config()
	return policy.Build(policy.Inputs{Cfg: cfg, Data: a.Data}, a.targets(cfg))
}

// Apply writes the policies. With offline set, the hives of signed-out
// protected users are loaded for a moment (used by install and apply-once,
// never by the service loop, so a sign-in is never blocked).
func (a *Agent) Apply(offline bool) policy.Result {
	a.applyMu.Lock()
	defer a.applyMu.Unlock()
	cfg := a.Config()
	desired := policy.Build(policy.Inputs{Cfg: cfg, Data: a.Data}, a.targets(cfg))
	return a.applyLocked(desired, offline)
}

// Remove removes or restores everything the agent wrote.
func (a *Agent) Remove(offline bool) policy.Result {
	a.applyMu.Lock()
	defer a.applyMu.Unlock()
	return a.applyLocked(nil, offline)
}

func (a *Agent) applyLocked(desired []policy.Entry, offline bool) policy.Result {
	reg := a.Sys.Reg()
	alias := policy.Alias{Registry: reg, Map: map[string]string{}}
	if offline {
		roots := map[string]bool{}
		for _, e := range desired {
			roots[e.Root] = true
		}
		for _, r := range a.st.Policy.Records {
			roots[r.Root] = true
		}
		for root := range roots {
			sid, ok := strings.CutPrefix(root, `HKU\`)
			if !ok || reg.RootAvailable(root) || a.Sys.SignedIn(sid) {
				continue
			}
			mounted, unmount, err := a.Sys.MountHive(sid)
			if err != nil {
				if desired == nil && errors.Is(err, core.ErrNoProfile) {
					// The profile was deleted: nothing of ours is left there.
					a.forgetRoot(root)
					continue
				}
				a.logf("could not load the profile of %s: %v", sid, err)
				continue
			}
			alias.Map[root] = mounted
			defer func(sid string) {
				if err := unmount(); err != nil {
					a.logf("could not unload the profile of %s: %v", sid, err)
				}
			}(sid)
		}
	}
	res := policy.Apply(alias, &a.st.Policy, desired)
	// Linux: the registry view is rendered to the browsers' policy files.
	if f, ok := reg.(interface{ Flush() error }); ok {
		if err := f.Flush(); err != nil {
			res.Errors = append(res.Errors, "write policy files: "+err.Error())
		}
	}
	a.last = res
	a.st.LastApply = a.now()
	if err := core.SaveJSON(a.Paths.State, &a.st); err != nil {
		res.Errors = append(res.Errors, "save state: "+err.Error())
	}
	for _, e := range res.Errors {
		a.logf("policy: %s", e)
	}
	return res
}

// forgetRoot drops the records of a root that no longer exists.
func (a *Agent) forgetRoot(root string) {
	var recs []policy.Record
	for _, r := range a.st.Policy.Records {
		if !strings.EqualFold(r.Root, root) {
			recs = append(recs, r)
		}
	}
	a.st.Policy.Records = recs
	var keys []policy.KeyRef
	for _, k := range a.st.Policy.CreatedKeys {
		if !strings.EqualFold(k.Root, root) {
			keys = append(keys, k)
		}
	}
	a.st.Policy.CreatedKeys = keys
}

// Detect refreshes the list of installed browsers.
func (a *Agent) Detect() []detect.Found {
	var f []detect.Found
	if d, ok := a.Sys.(interface {
		DetectBrowsers(core.UnsupportedList) []detect.Found
	}); ok {
		f = d.DetectBrowsers(a.Data.Unsupported) // Linux
	} else {
		f = detect.Detect(a.Sys.DetectEnv(), a.Data.Unsupported)
	}
	a.foundMu.Lock()
	a.found = f
	a.foundMu.Unlock()
	return f
}

// ActiveWithin is how recent an extension report must be to call a browser active.
const ActiveWithin = 15 * time.Minute

// BrowserStatus is the parent page's list of browsers: for each supported
// browser and protected account, "active" only when that browser's extension
// reported in the last 15 minutes; "set" when the policies are written but no
// report came; "waiting" when the account's hive was not loaded yet. Browsers
// we cannot protect are "closed".
func (a *Agent) BrowserStatus() []api.BrowserStatus {
	a.foundMu.Lock()
	found := append([]detect.Found(nil), a.found...)
	a.foundMu.Unlock()
	cfg := a.Config()
	a.applyMu.Lock()
	applied := map[string]bool{}
	for _, r := range a.st.Policy.Records {
		applied[r.Root] = true
	}
	a.applyMu.Unlock()
	now := a.Local()
	var supported, closed []api.BrowserStatus
	seen := map[string]bool{}
	for _, f := range found {
		if !f.Supported {
			key := "x|" + strings.ToLower(f.Path)
			if !seen[key] {
				seen[key] = true
				closed = append(closed, api.BrowserStatus{Name: f.Name, Path: f.Path, State: "closed"})
			}
			continue
		}
		for _, acct := range cfg.Accounts {
			key := f.ID + "|" + acct.SID
			if seen[key] {
				continue
			}
			seen[key] = true
			root := policy.RootForSID(acct.SID)
			if cfg.Mode == core.ModeAll {
				root = "HKLM"
			}
			b := api.BrowserStatus{Name: f.Name, Path: f.Path, State: "waiting"}
			if len(cfg.Accounts) > 1 {
				b.Account = acct.Name
			}
			if applied[root] {
				b.State = "set"
			}
			if t, ok := a.Store.LastSeen(acct.SID, f.ID); ok && now.Sub(t) <= ActiveWithin {
				b.State = "active"
				b.AgoMin = int(now.Sub(t) / time.Minute)
			}
			supported = append(supported, b)
		}
	}
	sort.SliceStable(supported, func(i, j int) bool { return supported[i].Name < supported[j].Name })
	sort.SliceStable(closed, func(i, j int) bool { return closed[i].Name < closed[j].Name })
	return append(supported, closed...)
}

// LastResult returns the last apply's counts.
func (a *Agent) LastResult() (policy.Result, time.Time, int) {
	a.applyMu.Lock()
	defer a.applyMu.Unlock()
	return a.last, a.st.LastApply, len(a.st.Policy.Records)
}

// Server returns the HTTP server for the extension and the parent page.
func (a *Agent) Server() *api.Server {
	cfg := a.Config()
	return &api.Server{
		Port: cfg.Port, Bands: &a.Data.Bands, Store: a.Store, Lock: a.Lock, Now: a.Local,
		Config: a.Config, Save: a.SaveConfig, Browsers: a.BrowserStatus,
		WholeComputer: wholeComputer(a.Sys),
		LocalAccounts: func() []api.LocalAccount {
			l, err := a.Sys.LocalAccounts()
			if err != nil {
				a.logf("list accounts: %v", err)
			}
			return l
		},
	}
}

// Intervals of the service loop.
const (
	ApplyEvery = 5 * time.Minute
	GuardEvery = 3 * time.Second
	SaveEvery  = time.Minute
)

// Run is the service loop. A value on logon means a user signed in.
func (a *Agent) Run(ctx context.Context, logon <-chan string) {
	a.Detect()
	r := a.Apply(false)
	a.logf("started: %d written, %d unchanged, %d pending, %d errors", r.Written, r.Unchanged, r.Pending, len(r.Errors))
	applyT := time.NewTicker(ApplyEvery)
	guardT := time.NewTicker(GuardEvery)
	saveT := time.NewTicker(SaveEvery)
	defer applyT.Stop()
	defer guardT.Stop()
	defer saveT.Stop()
	var retry <-chan time.Time
	lastPrune := a.now()
	for {
		select {
		case <-ctx.Done():
			_ = a.Store.Save()
			return
		case <-applyT.C:
			a.Detect()
			if r := a.Apply(false); r.Written+r.Removed > 0 || len(r.Errors) > 0 {
				a.logf("loop: %d written, %d removed, %d pending, %d errors", r.Written, r.Removed, r.Pending, len(r.Errors))
			}
		case sid := <-logon:
			if a.Protected(sid) || a.Config().Mode == core.ModeAll {
				r := a.Apply(false)
				a.logf("sign-in: %d written, %d pending", r.Written, r.Pending)
				retry = time.After(20 * time.Second)
			}
		case <-retry:
			retry = nil
			a.Detect()
			a.Apply(false)
		case <-guardT.C:
			if n, err := a.Guard.Tick(); n > 0 {
				a.logf("closed %d unsupported browser process(es)", n)
			} else if err != nil {
				a.logf("guard: %v", err)
			}
		case <-saveT.C:
			if err := a.Store.Save(); err != nil {
				a.logf("save usage: %v", err)
			}
			if a.now().Sub(lastPrune) > 24*time.Hour {
				a.Store.Prune(nil)
				lastPrune = a.now()
			}
		}
	}
}

// wholeComputer reports whether the system has machine-wide browser policies only (Linux).
func wholeComputer(sys System) bool {
	w, ok := sys.(interface{ WholeComputerOnly() bool })
	return ok && w.WholeComputerOnly()
}

// GuardList is the guard-list command's answer.
func (a *Agent) GuardList(all bool) ([]string, error) { return a.Guard.Report(all) }
