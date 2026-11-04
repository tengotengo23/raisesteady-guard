package agent

import (
	"testing"
	"time"

	"raisesteady-guard/agent/internal/core"
	"raisesteady-guard/agent/internal/guard"
	"raisesteady-guard/agent/internal/store"
)

func TestAppStateFor(t *testing.T) {
	d, err := core.LoadData()
	if err != nil {
		t.Fatal(err)
	}
	b, _ := d.Bands.Get("9-12") // video 60 min, bedtime 21:00-07:00
	zone := core.Zone(240)
	now := time.Date(2026, 11, 4, 15, 0, 0, 0, zone) // Wednesday
	st, _ := store.Open("", func() time.Time { return now })
	acct := core.Account{SID: kidOn, Settings: core.SettingsForBand(b)}

	s := AppStateFor(acct, d, st, now)
	if s.Stop != "" || s.LeftSec != 3600 || s.Modes["roblox"] != core.AppGame || s.Modes["discord"] != core.AppBlocked || s.Key != kidOn {
		t.Fatalf("defaults: %+v", s)
	}
	for i := 0; i < 360; i++ {
		st.AddGame(kidOn, 10)
	}
	if s = AppStateFor(acct, d, st, now); s.Stop != guard.StopUsed || s.LeftSec != 0 {
		t.Fatalf("used up: %+v", s)
	}
	st.AddExtra(kidOn, 15) // granted time counts for games too
	if s = AppStateFor(acct, d, st, now); s.Stop != "" || s.LeftSec != 900 {
		t.Fatalf("extra time: %+v", s)
	}
	acct.Homework = core.Window{On: true, Start: "14:00", End: "16:00"}
	if s = AppStateFor(acct, d, st, now); s.Stop != guard.StopHomework {
		t.Fatalf("homework: %+v", s)
	}
	if s = AppStateFor(acct, d, st, now.Add(7*time.Hour)); s.Stop != guard.StopBedtime {
		t.Fatalf("bedtime at 22:00: %+v", s)
	}
	acct.PauseUntil = now.Add(time.Hour).Format(time.RFC3339)
	if s = AppStateFor(acct, d, st, now); s.Stop != guard.StopPaused {
		t.Fatalf("paused wins: %+v", s)
	}
	g := 0
	acct.GameMin = &g
	acct.PauseUntil, acct.Homework = "", core.Window{}
	if s = AppStateFor(acct, d, st, now.Add(-3*time.Hour)); s.Stop != guard.StopUsed {
		t.Fatalf("game time 0 (only the 15 granted minutes, all used): %+v", s)
	}
}

func TestAppRulesAccountByMode(t *testing.T) {
	sys, _, d, p := setup(t)
	a := &Agent{Sys: sys, Data: d, Paths: p}
	b, _ := d.Bands.Get("13-15")
	a.cfg = core.Config{Mode: core.ModeAll, Accounts: []core.Account{{SID: core.AllSID, Settings: core.SettingsForBand(b)}}}
	st, _ := store.Open("", nil)
	a.Store = st
	r := appRules{a}
	s, ok := r.State(kidOn)
	if !ok || s.Key != core.AllSID || s.Modes["discord"] != core.AppAllowed {
		t.Fatalf("everyone mode: %+v %v", s, ok)
	}
	a.cfg = core.Config{Mode: core.ModeUsers, Accounts: []core.Account{{SID: kidOn, Settings: core.SettingsForBand(b)}}}
	if _, ok := r.State(kidOff); ok {
		t.Fatal("an account that is not protected has no rules")
	}
	r.AddGame(kidOn, 3)
	r.Closed(kidOn, "roblox")
	if sec, _ := st.GameToday(kidOn); sec != 3 || !r.Warn(kidOn) || r.Warn(kidOn) {
		t.Fatal("rules write to the store")
	}
}
