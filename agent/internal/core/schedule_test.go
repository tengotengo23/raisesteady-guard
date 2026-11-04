package core

import (
	"encoding/json"
	"testing"
	"time"
)

// 2026-11-02 is a Monday.
func at(day, h, m int) time.Time { return time.Date(2026, 11, day, h, m, 0, 0, Zone(240)) }

func weekendSettings() Settings {
	return Settings{
		Band: "9-12", BrowseMin: 120, VideoMin: 60, Bedtime: Bedtime{Start: "21:00", End: "07:00"},
		Weekend:  DayRule{On: true, BrowseMin: 180, VideoMin: 90, Bedtime: Bedtime{Start: "22:30", End: "09:00"}},
		Homework: Window{On: true, Start: "16:00", End: "18:00"},
	}
}

func TestWeekendLimits(t *testing.T) {
	s := weekendSettings()
	for _, c := range []struct {
		t             time.Time
		browse, video int
	}{
		{at(6, 12, 0), 120, 60}, // Friday: school day
		{at(7, 12, 0), 180, 90}, // Saturday
		{at(8, 12, 0), 180, 90}, // Sunday
		{at(9, 12, 0), 120, 60}, // Monday
	} {
		if b, v := s.LimitsOn(c.t); b != c.browse || v != c.video {
			t.Errorf("%s: %d/%d, want %d/%d", c.t.Weekday(), b, v, c.browse, c.video)
		}
	}
	s.Weekend.On = false
	if b, _ := s.LimitsOn(at(7, 12, 0)); b != 120 {
		t.Error("weekend off still changes Saturday")
	}
}

func TestWeekendBedtimeNights(t *testing.T) {
	s := weekendSettings()
	for _, c := range []struct {
		name string
		t    time.Time
		want bool
	}{
		{"Thursday night, school bedtime", at(5, 21, 30), true},
		{"Friday 21:30: Friday night is a weekend night", at(6, 21, 30), false},
		{"Friday 22:45", at(6, 22, 45), true},
		{"Saturday 08:00: the end of Friday night", at(7, 8, 0), true},
		{"Saturday 09:30", at(7, 9, 30), false},
		{"Sunday 08:00: the end of Saturday night", at(8, 8, 0), true},
		{"Sunday 21:30: Sunday night is a school night", at(8, 21, 30), true},
		{"Monday 06:30: the end of Sunday night", at(9, 6, 30), true},
		{"Monday 08:00", at(9, 8, 0), false},
	} {
		if got := s.BedtimeActiveAt(c.t); got != c.want {
			t.Errorf("%s: %v, want %v", c.name, got, c.want)
		}
	}
	if b := s.BedtimeNow(at(8, 8, 0)); b.End != "09:00" {
		t.Errorf("Sunday morning should show Saturday night's end: %+v", b)
	}
	if b := s.BedtimeNow(at(8, 12, 0)); b.Start != "21:00" {
		t.Errorf("Sunday afternoon should show the school bedtime: %+v", b)
	}
	if b := s.BedtimeNow(at(6, 12, 0)); b.Start != "22:30" {
		t.Errorf("Friday afternoon should show the weekend bedtime: %+v", b)
	}
	same := Settings{Bedtime: Bedtime{Start: "13:00", End: "14:00"}}
	if !same.BedtimeActiveAt(at(4, 13, 30)) || same.BedtimeActiveAt(at(4, 14, 0)) {
		t.Error("same-day window")
	}
}

func TestHomeworkAndPause(t *testing.T) {
	s := weekendSettings()
	for _, c := range []struct {
		t    time.Time
		want bool
	}{
		{at(4, 16, 30), true},  // Wednesday
		{at(4, 18, 0), false},  // the end is not inside
		{at(4, 15, 59), false}, // before
		{at(7, 16, 30), false}, // Saturday
		{at(8, 17, 0), false},  // Sunday
		{at(6, 16, 0), true},   // Friday
	} {
		if got := s.HomeworkActive(c.t); got != c.want {
			t.Errorf("%s %s: %v", c.t.Weekday(), c.t.Format("15:04"), got)
		}
	}
	s.Homework.On = false
	if s.HomeworkActive(at(4, 16, 30)) {
		t.Error("homework off")
	}
	now := at(4, 15, 0)
	s.PauseUntil = now.Add(30 * time.Minute).UTC().Format(time.RFC3339)
	if p, until := s.PausedAt(now); !p || until.Format("15:04") != "15:30" {
		t.Errorf("paused: %v %v", p, until)
	}
	if p, _ := s.PausedAt(now.Add(30 * time.Minute)); p {
		t.Error("the pause ends at its end time")
	}
	if !NextMorning(at(4, 1, 0)).Equal(at(4, 6, 0)) || !NextMorning(at(4, 20, 0)).Equal(at(5, 6, 0)) || !NextMorning(at(4, 6, 0)).Equal(at(5, 6, 0)) {
		t.Error("NextMorning")
	}
}

func TestOldConfigDefaults(t *testing.T) {
	d, err := LoadData()
	if err != nil {
		t.Fatal(err)
	}
	// A 0.2.2 account: none of the new fields.
	old := `{"sid":"S-1-5-21-1-2-3-1001","name":"Ana","token":"` + NewToken() + `","band":"9-12","browseMin":120,"videoMin":60,"bedtime":{"start":"21:00","end":"07:00"},"allow":[],"block":[],"focusMode":true,"adblock":true}`
	var a Account
	if err := json.Unmarshal([]byte(old), &a); err != nil {
		t.Fatal(err)
	}
	if err := a.Settings.Validate(&d.Bands); err != nil {
		t.Fatal(err)
	}
	s := a.Settings
	if s.Weekend.On || s.Homework.On || s.PauseUntil != "" || s.SearchesOff || s.GameMin != nil || s.Apps != nil {
		t.Fatalf("new fields not off: %+v", s)
	}
	if b, v := s.LimitsOn(at(7, 12, 0)); b != 120 || v != 60 {
		t.Error("old config: Saturday must keep the school-day minutes")
	}
	if s.GameMinutes(&d.Bands) != 60 {
		t.Errorf("game time default = band video minutes: %d", s.GameMinutes(&d.Bands))
	}
	modes := map[string]string{}
	for _, app := range d.Apps.Apps {
		modes[app.ID] = s.AppMode(app, &d.Bands)
	}
	if modes["roblox"] != AppGame || modes["steam"] != AppGame || modes["discord"] != AppBlocked || modes["whatsapp"] != AppBlocked || modes["telegram"] != AppBlocked {
		t.Errorf("9-12 defaults: %v", modes)
	}
	s.Band = "13-15"
	if s.AppMode(mustApp(t, d, "discord"), &d.Bands) != AppAllowed || s.AppMode(mustApp(t, d, "telegram"), &d.Bands) != AppBlocked {
		t.Error("13-15: Discord allowed, Telegram (18) blocked")
	}
	s.Apps = map[string]string{"discord": AppBlocked}
	if s.AppMode(mustApp(t, d, "discord"), &d.Bands) != AppBlocked {
		t.Error("the parent's choice wins")
	}
}

func mustApp(t *testing.T, d *Data, id string) PCApp {
	t.Helper()
	a, ok := d.Apps.Get(id)
	if !ok {
		t.Fatalf("no app %s", id)
	}
	return a
}

func TestValidateNewFields(t *testing.T) {
	d, _ := LoadData()
	ok := weekendSettings()
	if err := ok.Validate(&d.Bands); err != nil {
		t.Fatal(err)
	}
	g := 30
	for name, bad := range map[string]func(s *Settings){
		"homework end before start": func(s *Settings) { s.Homework = Window{On: true, Start: "18:00", End: "16:00"} },
		"weekend bedtime":           func(s *Settings) { s.Weekend.Bedtime.Start = "25:00" },
		"pause time":                func(s *Settings) { s.PauseUntil = "soon" },
		"game time":                 func(s *Settings) { g = 2000; s.GameMin = &g },
		"app mode":                  func(s *Settings) { s.Apps = map[string]string{"roblox": "sometimes"} },
		"app id":                    func(s *Settings) { s.Apps = map[string]string{"Rob lox": AppGame} },
	} {
		s := weekendSettings()
		bad(&s)
		if s.Validate(&d.Bands) == nil {
			t.Errorf("%s accepted", name)
		}
	}
	off := weekendSettings()
	off.Homework = Window{On: false, Start: "18:00", End: "16:00"}
	if off.Validate(&d.Bands) != nil {
		t.Error("an off homework window is not checked")
	}
}

func TestPCAppsData(t *testing.T) {
	d, _ := LoadData()
	if len(d.Apps.Apps) < 11 {
		t.Fatalf("pc-apps.json: %d apps", len(d.Apps.Apps))
	}
	seen := map[string]bool{}
	for _, a := range d.Apps.Apps {
		if !appIDRe.MatchString(a.ID) || seen[a.ID] || a.Name == "" || (a.Kind != "game" && a.Kind != "chat") {
			t.Errorf("bad app %+v", a)
		}
		seen[a.ID] = true
		if a.Kind == "chat" && (a.MinAge < 13 || a.Terms == "") {
			t.Errorf("%s: a chat app needs its minimum age and terms URL", a.ID)
		}
		if len(a.Exe) == 0 {
			t.Errorf("%s: no Windows exe names", a.ID)
		}
	}
}
