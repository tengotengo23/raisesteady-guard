package api

import (
	"strings"
	"testing"
	"time"

	"raisesteady-guard/agent/internal/update"
)

func TestParentShowsUpdateLine(t *testing.T) {
	r := newRig(t)
	checked := time.Date(2026, 11, 19, 8, 30, 0, 0, r.s.now().Location())
	st := update.Status{Version: "0.4.5", LastCheck: checked}
	r.s.Update = func() update.Status { return st }

	// Logged out: no line.
	if page := newBrowser(t, r).send("GET", "/parent", nil, "").Body.String(); strings.Contains(page, `id="update-line"`) {
		t.Fatal("update line shown before login")
	}

	b := newBrowser(t, r)
	if w := b.login("2468"); w.Code != 303 {
		t.Fatalf("login: %d", w.Code)
	}
	page := b.send("GET", "/parent", nil, "").Body.String()
	want := "Version 0.4.5. Last check for updates: 2026-11-19 08:30. Last update: not yet."
	if !strings.Contains(page, want) {
		t.Fatalf("page lacks %q", want)
	}
	if strings.Contains(page, "did not succeed") {
		t.Fatal("failing note shown while checks succeed")
	}

	st.Failing = true
	if page := b.send("GET", "/parent", nil, "").Body.String(); !strings.Contains(page, "The last checks for updates did not succeed.") {
		t.Fatal("failing note missing")
	}

	r.s.Update = nil
	if page := b.send("GET", "/parent", nil, "").Body.String(); strings.Contains(page, `id="update-line"`) {
		t.Fatal("update line shown without an updater")
	}
}
