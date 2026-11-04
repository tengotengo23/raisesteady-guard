package api

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"raisesteady-guard/agent/internal/core"
	"raisesteady-guard/agent/internal/store"
)

// P2-160: weekend, pause, homework, searches, PC apps and requests.

const self = "http://127.0.0.1:47615"

func (r *rig) setSettings(i int, fn func(s *core.Settings)) {
	r.mu.Lock()
	defer r.mu.Unlock()
	fn(&r.cfg.Accounts[i].Settings)
}

func TestStateWeekendPauseHomework(t *testing.T) {
	r := newRig(t)
	loc := time.FixedZone("UTC+04:00", 4*3600)
	r.now = time.Date(2026, 11, 7, 8, 0, 0, 0, loc) // Saturday 08:00
	r.setSettings(0, func(s *core.Settings) {
		s.BrowseMin, s.VideoMin, s.Bedtime = 90, 45, core.Bedtime{Start: "20:00", End: "07:00"}
		s.Weekend = core.DayRule{On: true, BrowseMin: 150, VideoMin: 60, Bedtime: core.Bedtime{Start: "21:30", End: "08:30"}}
		s.Homework = core.Window{On: true, Start: "16:00", End: "18:00"}
	})
	st := decodeState(t, r.do(req{path: "/v1/state", token: tokA}))
	if st.Limits.BrowseMin != 150 || st.Limits.VideoMin != 60 || !st.BedtimeActive || st.Bedtime.End != "08:30" {
		t.Fatalf("Saturday morning after Friday night: %+v", st)
	}
	if st.Schedule.Weekend == nil || st.Schedule.School.BrowseMin != 90 || st.Schedule.Homework == nil || st.Homework || st.Paused || !st.Searches {
		t.Fatalf("schedule: %+v", st)
	}
	r.now = time.Date(2026, 11, 4, 16, 30, 0, 0, loc) // Wednesday
	st = decodeState(t, r.do(req{path: "/v1/state", token: tokA}))
	if !st.Homework || st.HomeworkEnd != "18:00" || st.Limits.BrowseMin != 90 {
		t.Fatalf("Wednesday homework: %+v", st)
	}
	r.setSettings(0, func(s *core.Settings) { s.PauseUntil = r.now.Add(time.Hour).Format(time.RFC3339) })
	st = decodeState(t, r.do(req{path: "/v1/state", token: tokA}))
	if !st.Paused || st.PausedUntilHM != "17:30" {
		t.Fatalf("paused: %+v", st)
	}
	// The PIN at the paused page ends the pause.
	w := r.do(req{method: "POST", path: "/v1/grant", token: tokA, body: `{"pin":"1111","kind":"resume"}`})
	if w.Code != http.StatusForbidden {
		t.Fatalf("wrong PIN: %d", w.Code)
	}
	st = decodeState(t, r.do(req{method: "POST", path: "/v1/grant", token: tokA, body: `{"pin":"2468","kind":"resume"}`}))
	if st.Paused || r.cfg.Accounts[0].PauseUntil != "" {
		t.Fatal("resume with the PIN did not end the pause")
	}
	// Luka (no new settings): unchanged.
	st = decodeState(t, r.do(req{path: "/v1/state", token: tokB}))
	if st.Homework || st.Paused || st.Schedule.Weekend != nil || st.Schedule.Homework != nil {
		t.Fatalf("old settings: %+v", st)
	}
}

func TestUsageSearchesAndSwitch(t *testing.T) {
	r := newRig(t)
	body := `{"browser":"chrome","browseSec":0,"searches":{"Volcano facts":1}}`
	decodeState(t, r.do(req{method: "POST", path: "/v1/usage", token: tokA, body: body}))
	if d := r.s.Store.Detail(kidA, "2026-11-03"); len(d.Searches) != 1 || d.Searches[0].Key != "volcano facts" {
		t.Fatalf("search not kept: %+v", d)
	}
	r.setSettings(1, func(s *core.Settings) { s.SearchesOff = true })
	st := decodeState(t, r.do(req{method: "POST", path: "/v1/usage", token: tokB, body: body}))
	if d := r.s.Store.Detail(kidB, "2026-11-03"); len(d.Searches) != 0 || st.Searches {
		t.Fatalf("switched off, yet kept: %+v", d)
	}
}

func TestChildRequests(t *testing.T) {
	r := newRig(t)
	for _, bad := range []string{
		`{"kind":"site","domain":"not a domain"}`,
		`{"kind":"other"}`,
		`{"kind":"time","note":"` + strings.Repeat("x", 600) + `"}`,
	} {
		if w := r.do(req{method: "POST", path: "/v1/request", token: tokA, body: bad}); w.Code != http.StatusBadRequest {
			t.Errorf("%s: %d", bad, w.Code)
		}
	}
	if w := r.do(req{method: "POST", path: "/v1/request", body: `{"kind":"time"}`}); w.Code != http.StatusUnauthorized {
		t.Fatalf("no token: %d", w.Code)
	}
	var out struct{ OK, Dup bool }
	w := r.do(req{method: "POST", path: "/v1/request", token: tokA, body: `{"kind":"site","domain":"https://www.Khanacademy.org/math","note":"for maths"}`})
	_ = json.Unmarshal(w.Body.Bytes(), &out)
	if w.Code != 200 || !out.OK || out.Dup {
		t.Fatalf("request: %d %s", w.Code, w.Body)
	}
	w = r.do(req{method: "POST", path: "/v1/request", token: tokA, body: `{"kind":"site","domain":"khanacademy.org"}`})
	_ = json.Unmarshal(w.Body.Bytes(), &out)
	if !out.Dup {
		t.Fatal("the same request twice in a row is a duplicate")
	}
	r.do(req{method: "POST", path: "/v1/request", token: tokB, body: `{"kind":"time","note":"film night"}`})
	// At most 10 requests a minute per account (Ana has sent 2).
	for i := 0; i < 8; i++ {
		if w := r.do(req{method: "POST", path: "/v1/request", token: tokA, body: `{"kind":"time"}`}); w.Code != 200 {
			t.Fatalf("request %d: %d", i+3, w.Code)
		}
	}
	if w := r.do(req{method: "POST", path: "/v1/request", token: tokA, body: `{"kind":"time"}`}); w.Code != http.StatusTooManyRequests {
		t.Fatalf("11th request in a minute: %d", w.Code)
	}
	r.now = r.now.Add(time.Minute)
	if loc := r.s.Store.Requests(kidA); len(loc) != 2 || loc[0].Kind != "time" {
		t.Fatalf("site then time (repeats folded): %+v", loc)
	}
	r.s.Store.TakeRequest(kidA, r.s.Store.Requests(kidA)[0].ID) // leave Ana's site request for the answers below

	b := &browser{r: r, t: t, cookies: map[string]*http.Cookie{}}
	if strings.Contains(b.send("GET", "/parent", nil, "").Body.String(), "khanacademy") {
		t.Fatal("requests shown before login")
	}
	b.login("2468")
	page := b.send("GET", "/parent?lang=en", nil, "").Body.String()
	iReq, iPause := strings.Index(page, "Requests from your child"), strings.Index(page, "Pause the internet")
	if iReq < 0 || iPause < 0 || iReq > iPause || strings.Index(page, "Age and daily time") < iPause {
		t.Fatal("requests must come first after login")
	}
	for _, want := range []string{"Ana asks you to allow khanacademy.org.", "Note: “for maths”", "Luka asks for more time today.", "Note: “film night”", `value="allow">Allow this site`, `value="15">15 min`, `value="30">30 min`, `value="no">No`} {
		if !strings.Contains(page, want) {
			t.Errorf("page lacks %q", want)
		}
	}
	reqA, reqB := r.s.Store.Requests(kidA)[0], r.s.Store.Requests(kidB)[0]
	answer := func(a, id, ans string) string {
		w := b.send("POST", "/parent/request", url.Values{"csrf": {b.csrf()}, "a": {a}, "id": {id}, "answer": {ans}, "lang": {"en"}}, self)
		return w.Header().Get("Location")
	}
	if loc := answer("0", reqA.ID, "30"); !strings.Contains(loc, "err=bad_input") {
		t.Fatalf("minutes for a site request: %s", loc)
	}
	if loc := answer("0", reqA.ID, "allow"); !strings.Contains(loc, "ok=req_done") || !containsStr(r.cfg.Accounts[0].Allow, "khanacademy.org") {
		t.Fatalf("allow: %s %v", loc, r.cfg.Accounts[0].Allow)
	}
	if loc := answer("0", reqA.ID, "allow"); !strings.Contains(loc, "err=req_gone") {
		t.Fatalf("answered twice: %s", loc)
	}
	if loc := answer("1", reqB.ID, "15"); !strings.Contains(loc, "ok=req_done") || r.s.Store.Today(kidB).ExtraMin != 15 {
		t.Fatalf("time: %s %d", loc, r.s.Store.Today(kidB).ExtraMin)
	}
	r.do(req{method: "POST", path: "/v1/request", token: tokB, body: `{"kind":"time"}`})
	if loc := answer("1", r.s.Store.Requests(kidB)[0].ID, "no"); !strings.Contains(loc, "ok=req_done") || len(r.s.Store.Requests(kidB)) != 0 || r.s.Store.Today(kidB).ExtraMin != 15 {
		t.Fatalf("no: %s", loc)
	}
	if strings.Contains(b.send("GET", "/parent?lang=en", nil, "").Body.String(), "asks") {
		t.Fatal("answered requests still shown")
	}
}

func TestParentPause(t *testing.T) {
	r := newRig(t)
	b := &browser{r: r, t: t, cookies: map[string]*http.Cookie{}}
	b.login("2468")
	pause := func(dur string) string {
		w := b.send("POST", "/parent/pause", url.Values{"csrf": {b.csrf()}, "a": {"0"}, "dur": {dur}, "lang": {"en"}}, self)
		return w.Header().Get("Location")
	}
	if loc := pause("45"); !strings.Contains(loc, "err=bad_input") || r.cfg.Accounts[0].PauseUntil != "" {
		t.Fatalf("odd duration accepted: %s", loc)
	}
	if loc := pause("30"); !strings.Contains(loc, "ok=pause_saved") {
		t.Fatal(loc)
	}
	if p, until := r.cfg.Accounts[0].PausedAt(r.now); !p || until.Sub(r.now) != 30*time.Minute {
		t.Fatalf("30 min: %v %v", p, until)
	}
	page := b.send("GET", "/parent?lang=en", nil, "").Body.String()
	if !strings.Contains(page, "Paused until 15:30.") || !strings.Contains(page, `value="resume"`) {
		t.Fatal("paused state not on the page")
	}
	pause("tomorrow")
	if _, until := r.cfg.Accounts[0].PausedAt(r.now); until.Format("2006-01-02 15:04") != "2026-11-04 06:00" {
		t.Fatalf("until tomorrow: %v", until)
	}
	if r.cfg.Accounts[1].PauseUntil != "" {
		t.Fatal("the pause is per account")
	}
	pause("resume")
	if r.cfg.Accounts[0].PauseUntil != "" {
		t.Fatal("resume")
	}
	if !strings.Contains(b.send("GET", "/parent?lang=ka", nil, "").Body.String(), "ინტერნეტის შეჩერება") {
		t.Fatal("Georgian pause card")
	}
	if w := b.send("GET", "/favicon.ico", nil, ""); w.Code != http.StatusNoContent {
		t.Fatalf("favicon: %d (a 404 is a console error on the parent page)", w.Code)
	}
}

func TestParentSavesWeekendHomeworkSearches(t *testing.T) {
	r := newRig(t)
	b := &browser{r: r, t: t, cookies: map[string]*http.Cookie{}}
	b.login("2468")
	page := b.send("GET", "/parent?lang=en", nil, "").Body.String()
	// Off by default, the weekend fields start with the school-day values (6-8: 90/45, 20:00-07:00).
	for _, want := range []string{`name="weekendOn" aria-describedby="weekend-hint">`, `id="wBrowseMin" name="wBrowseMin" type="number" inputmode="numeric" min="5" max="720" step="5" value="90"`, `id="wBedStart" name="wBedStart" type="time" value="20:00"`, `name="homeworkOn" aria-describedby="homework-hint">`, `name="searches" aria-describedby="searches-hint" checked`} {
		if !strings.Contains(page, want) {
			t.Errorf("page lacks %q", want)
		}
	}
	form := url.Values{"a": {"0"}, "band": {"6-8"}, "browseMin": {"90"}, "videoMin": {"45"}, "bedStart": {"20:00"}, "bedEnd": {"07:00"},
		"weekendOn": {"on"}, "wBrowseMin": {"150"}, "wVideoMin": {"60"}, "wBedStart": {"21:00"}, "wBedEnd": {"08:30"},
		"homeworkOn": {"on"}, "hwStart": {"16:00"}, "hwEnd": {"18:00"}, "focusMode": {"on"}, "familyDns": {"on"}}
	form.Set("csrf", b.csrf())
	if loc := b.send("POST", "/parent/save", form, self).Header().Get("Location"); !strings.Contains(loc, "ok=saved") {
		t.Fatal(loc)
	}
	st := r.cfg.Accounts[0].Settings
	if !st.Weekend.On || st.Weekend.BrowseMin != 150 || st.Weekend.Bedtime.End != "08:30" || !st.Homework.On || st.Homework.End != "18:00" || !st.SearchesOff {
		t.Fatalf("saved: %+v", st)
	}
	form.Set("hwEnd", "15:00")
	form.Set("searches", "on")
	form.Set("csrf", b.csrf())
	if loc := b.send("POST", "/parent/save", form, self).Header().Get("Location"); !strings.Contains(loc, "err=bad_input") || r.cfg.Accounts[0].SearchesOff != true {
		t.Fatalf("a homework end before its start was saved: %s", loc)
	}
	// Off: a bad window is not checked, and the last good one is kept.
	form.Del("homeworkOn")
	form.Del("weekendOn")
	form.Set("csrf", b.csrf())
	b.send("POST", "/parent/save", form, self)
	st = r.cfg.Accounts[0].Settings
	if st.Homework.On || st.Homework.End != "18:00" || st.Weekend.On || st.Weekend.BrowseMin != 150 || st.SearchesOff {
		t.Fatalf("switched off: %+v", st)
	}
}

func TestParentApps(t *testing.T) {
	r := newRig(t)
	b := &browser{r: r, t: t, cookies: map[string]*http.Cookie{}}
	b.login("2468")
	page := b.send("GET", "/parent?lang=en", nil, "").Body.String()
	for _, want := range []string{"PC games and apps", `id="gameMin" name="gameMin" type="number" inputmode="numeric" min="0" max="720" step="5" value="45"`, `<option value="game" selected>Game time</option>`, "Minimum age in its terms: 13", "Closing Steam does not always close a game started from it.", "Extra minutes you give, with the PIN or as an answer to a request, count for games too."} {
		if !strings.Contains(page, want) {
			t.Errorf("page lacks %q", want)
		}
	}
	if strings.Contains(page, "In this mode the PC apps") {
		t.Error("the every-account note shows only in whole-computer mode")
	}
	r.s.WholeComputer = true
	if !strings.Contains(b.send("GET", "/parent?lang=en", nil, "").Body.String(), "In this mode the PC apps, the pause and the searches apply to every account, yours too.") {
		t.Error("whole-computer mode must say apps, pause and searches apply to every account")
	}
	r.s.WholeComputer = false
	form := url.Values{"a": {"0"}, "gameMin": {"45"}, "app_roblox": {"blocked"}, "app_discord": {"blocked"}, "csrf": {b.csrf()}}
	if loc := b.send("POST", "/parent/apps", form, self).Header().Get("Location"); !strings.Contains(loc, "ok=saved") {
		t.Fatal(loc)
	}
	st := r.cfg.Accounts[0].Settings
	if st.GameMin != nil || len(st.Apps) != 1 || st.Apps["roblox"] != core.AppBlocked {
		t.Fatalf("only changes from the defaults are kept: %+v %v", st.Apps, st.GameMin)
	}
	form = url.Values{"a": {"0"}, "gameMin": {"20"}, "app_roblox": {"sometimes"}, "csrf": {b.csrf()}}
	if loc := b.send("POST", "/parent/apps", form, self).Header().Get("Location"); !strings.Contains(loc, "err=bad_input") {
		t.Fatal("bad mode accepted")
	}
	form.Set("app_roblox", "game")
	form.Set("csrf", b.csrf())
	b.send("POST", "/parent/apps", form, self)
	if st := r.cfg.Accounts[0].Settings; st.GameMin == nil || *st.GameMin != 20 || st.Apps != nil {
		t.Fatalf("game minutes: %+v", st)
	}
}

func TestReportDayPicker(t *testing.T) {
	r := newRig(t)
	r.s.Store.AddUsage(kidA, storeUsage(map[string]int{"dinosaurs": 2}))
	r.s.Store.AddGame(kidA, 10)
	r.s.Store.AddAppClosed(kidA, "roblox")
	r.now = r.now.Add(24 * time.Hour)
	b := &browser{r: r, t: t, cookies: map[string]*http.Cookie{}}
	b.login("2468")
	today := b.send("GET", "/parent?lang=en", nil, "").Body.String()
	if !strings.Contains(today, "Searches on 2026-11-04") || strings.Contains(today, "dinosaurs") {
		t.Fatal("today is picked by default and has no searches")
	}
	y := b.send("GET", "/parent?lang=en&a=0&day=2026-11-03", nil, "").Body.String()
	for _, want := range []string{"Searches on 2026-11-03", "<span>dinosaurs</span> <span class=\"muted\">2</span>", "PC apps closed on 2026-11-03", "<span>Roblox</span>", `day=2026-11-03&amp;lang=en#day-h" aria-current="date"`, "PC games"} {
		if !strings.Contains(y, want) {
			t.Errorf("picked day lacks %q", want)
		}
	}
	if odd := b.send("GET", "/parent?lang=en&day=2020-01-01", nil, "").Body.String(); !strings.Contains(odd, "Searches on 2026-11-04") {
		t.Error("a day outside the 7 days falls back to today")
	}
}

func storeUsage(searches map[string]int) store.Usage {
	return store.Usage{Browser: "chrome", Searches: searches}
}

// P2-161: the topic filter switch. Old config files load with it on; the parent page shows it checked,
// saving without it turns it off (the state says so), and blocked topic pages count under their own reason.
func TestTopicFilterSwitch(t *testing.T) {
	r := newRig(t)
	var old core.Settings
	if err := json.Unmarshal([]byte(`{"band":"9-12","focusMode":true}`), &old); err != nil || old.TopicsOff {
		t.Fatalf("an old config must load with the topic filter on: %+v %v", old, err)
	}
	if st := decodeState(t, r.do(req{path: "/v1/state", token: tokA})); !st.Topics {
		t.Fatal("state: topics should be on by default")
	}
	b := &browser{r: r, t: t, cookies: map[string]*http.Cookie{}}
	b.login("2468")
	page := b.send("GET", "/parent?lang=en", nil, "").Body.String()
	for _, want := range []string{`name="topics" aria-describedby="topics-hint" checked`, "Sexuality and gender identity topics", "Helplines stay open."} {
		if !strings.Contains(page, want) {
			t.Errorf("page lacks %q", want)
		}
	}
	if ka := b.send("GET", "/parent?lang=ka", nil, "").Body.String(); !strings.Contains(ka, "სექსუალობისა და გენდერული იდენტობის თემები") {
		t.Error("Georgian label")
	}
	if ru := b.send("GET", "/parent?lang=ru", nil, "").Body.String(); !strings.Contains(ru, "Темы сексуальности и гендерной идентичности") {
		t.Error("Russian label")
	}
	form := url.Values{"a": {"0"}, "band": {"6-8"}, "browseMin": {"90"}, "videoMin": {"45"}, "bedStart": {"20:00"}, "bedEnd": {"07:00"}, "focusMode": {"on"}, "familyDns": {"on"}}
	form.Set("csrf", b.csrf())
	if loc := b.send("POST", "/parent/save", form, self).Header().Get("Location"); !strings.Contains(loc, "ok=saved") {
		t.Fatal(loc)
	}
	if !r.cfg.Accounts[0].TopicsOff {
		t.Fatal("unchecked switch should turn the filter off")
	}
	if st := decodeState(t, r.do(req{path: "/v1/state", token: tokA})); st.Topics {
		t.Fatal("state: topics should be off now")
	}
	form.Set("topics", "on")
	form.Set("csrf", b.csrf())
	b.send("POST", "/parent/save", form, self)
	if r.cfg.Accounts[0].TopicsOff {
		t.Fatal("checked switch should turn it on again")
	}
	if w := r.do(req{method: "POST", path: "/v1/event", token: tokA, body: `{"type":"blocked","reason":"topic","domain":"google.com"}`}); w.Code != 200 {
		t.Fatalf("topic event: %d", w.Code)
	}
	if rep, _ := r.s.Store.Report(kidA, 1); len(rep) == 0 || rep[0].ByReason["topic"] != 1 {
		t.Fatalf("topic count: %+v", rep)
	}
}
