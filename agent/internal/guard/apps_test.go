package guard

import (
	"strings"
	"testing"
	"time"

	"raisesteady-guard/agent/internal/core"
)

func apps(t *testing.T) []core.PCApp {
	t.Helper()
	d, err := core.LoadData()
	if err != nil {
		t.Fatal(err)
	}
	return d.Apps.Apps
}

func TestClassifyApp(t *testing.T) {
	l := apps(t)
	for _, c := range []struct {
		path, product string
		linux         bool
		want          string
	}{
		{`C:\Users\kid\AppData\Local\Roblox\Versions\version-1\RobloxPlayerBeta.exe`, "Roblox", false, "roblox"},
		{`C:\Users\kid\AppData\Local\Discord\app-1.0.9\Discord.exe`, "Discord", false, "discord"},
		{`C:\Users\kid\Desktop\homework.exe`, "Discord", false, "discord"}, // renamed: the product name still says Discord
		{`C:\Program Files (x86)\Steam\steam.exe`, "Steam", false, "steam"},
		{`C:\Program Files\WindowsApps\ROBLOXCORPORATION.ROBLOX_2.650.1.0_x64__55nm5eh3cm0pr\Windows10Universal.exe`, "", false, "roblox"},
		{`C:\Riot Games\League of Legends\LeagueClient.exe`, "", false, "riot"},
		{`C:\Program Files\Epic Games\Fortnite\FortniteGame\Binaries\Win64\FortniteClient-Win64-Shipping.exe`, "", false, "epic-fortnite"},
		{`C:\Users\kid\AppData\Roaming\Telegram Desktop\Telegram.exe`, "Telegram Desktop", false, "telegram"},
		{`C:\Program Files\Google\Chrome\Application\chrome.exe`, "Google Chrome", false, ""},
		{`C:\Windows\notepad.exe`, "Microsoft® Windows® Operating System", false, ""},
		{"/usr/share/discord/Discord", "", true, "discord"},
		{"flatpak:com.valvesoftware.Steam", "", true, "steam"},
		{"flatpak-user:org.telegram.desktop", "", true, "telegram"},
		{"flatpak-claim:com.discordapp.Discord:/app/discord/Discord", "", true, "discord"},
		{"/snap/telegram-desktop/123/usr/bin/telegram-desktop", "", true, "telegram"},
		{"unverified:/home/kid/.local/bin/minecraft-launcher", "", true, "minecraft"},
		{"flatpak:org.gnome.Calculator", "", true, ""},
		{"/usr/bin/bash", "", true, ""},
	} {
		got := ""
		if a := ClassifyApp(l, c.path, c.product, c.linux); a != nil {
			got = a.ID
		}
		if got != c.want {
			t.Errorf("%s: %q, want %q", c.path, got, c.want)
		}
	}
}

type fakeRules struct {
	st      map[string]AppState
	game    map[string]int
	warned  map[string]bool
	closed  []string
	lookups int
}

func (f *fakeRules) State(sid string) (AppState, bool) {
	f.lookups++
	s, ok := f.st[sid]
	return s, ok
}
func (f *fakeRules) AddGame(key string, sec int) { f.game[key] += sec }
func (f *fakeRules) Warn(key string) bool {
	if f.warned[key] {
		return false
	}
	f.warned[key] = true
	return true
}
func (f *fakeRules) Closed(key, app string) { f.closed = append(f.closed, key+":"+app) }

func TestTickCountsAndClosesApps(t *testing.T) {
	kid, kid2, parent := "S-1-5-21-1-2-3-1001", "S-1-5-21-1-2-3-1002", "S-1-5-21-1-2-3-1000"
	now := time.Date(2026, 11, 4, 16, 0, 0, 0, time.UTC)
	roblox := `C:\Users\kid\AppData\Local\Roblox\Versions\v1\RobloxPlayerBeta.exe`
	discord := `C:\Users\kid\AppData\Local\Discord\app-1\Discord.exe`
	fs := &fakeSys{procs: []Proc{
		{PID: 1, Session: 2, SID: kid, Path: roblox},
		{PID: 2, Session: 2, SID: kid, Path: discord},
		{PID: 3, Session: 1, SID: parent, Path: roblox},
		{PID: 4, Session: 3, SID: kid2, Path: roblox},
	}}
	modes := map[string]string{"roblox": core.AppGame, "discord": core.AppBlocked}
	fr := &fakeRules{
		st: map[string]AppState{
			kid:  {Key: kid, Modes: modes, LeftSec: 600},
			kid2: {Key: kid2, Modes: modes, Stop: StopHomework, LeftSec: 600},
		},
		game: map[string]int{}, warned: map[string]bool{},
	}
	g := &Guard{Sys: fs, List: list(t), Apps: apps(t), Rules: fr,
		Protected: func(s string) bool { return s == kid || s == kid2 }, Now: func() time.Time { return now }}

	// Tick 1: Discord (blocked) closes; kid2's Roblox closes in homework time;
	// kid's Roblox runs. The first tick counts no time.
	n, err := g.Tick()
	if err != nil || n != 2 || len(fs.killed) != 2 || fs.killed[0] != 2 || fs.killed[1] != 4 {
		t.Fatalf("tick 1 closed %d: %v", n, fs.killed)
	}
	if fr.game[kid] != 0 {
		t.Fatal("the first tick counts no game time")
	}
	if strings.Join(fr.closed, ",") != kid+":discord,"+kid2+":roblox" {
		t.Fatalf("closed for the report: %v", fr.closed)
	}
	if len(fs.msgs) != 2 || !strings.Contains(fs.msgs[0], "Discord") || !strings.Contains(fs.msgs[1], "საშინაო") {
		t.Fatalf("messages: %v", fs.msgs)
	}
	if fr.lookups != 2 {
		t.Fatalf("rules looked up once per account per tick, got %d", fr.lookups)
	}

	// Tick 2, 3 s later: 3 s of game time; within a minute no new message.
	now = now.Add(3 * time.Second)
	fs.killed = nil
	g.Tick()
	if fr.game[kid] != 3 || fr.game[kid2] != 0 {
		t.Fatalf("game time: %v", fr.game)
	}
	if len(fs.msgs) != 2 {
		t.Fatalf("message repeated within a minute: %v", fs.msgs)
	}

	// 5 minutes left: one warning, once.
	fr.st[kid] = AppState{Key: kid, Modes: modes, LeftSec: 290}
	now = now.Add(3 * time.Second)
	g.Tick()
	now = now.Add(3 * time.Second)
	g.Tick()
	warns := 0
	for _, m := range fs.msgs {
		if strings.Contains(m, "5 წუთზე ნაკლები") {
			warns++
		}
	}
	if warns != 1 {
		t.Fatalf("one 5-minute warning expected: %v", fs.msgs)
	}

	// Time used up: Roblox closes; a long gap (sleep) counts nothing.
	fr.st[kid] = AppState{Key: kid, Modes: modes, Stop: StopUsed}
	before := fr.game[kid]
	now = now.Add(5 * time.Minute)
	fs.killed = nil
	g.Tick()
	if len(fs.killed) != 3 || fr.game[kid] != before {
		t.Fatalf("used up: killed %v, game %d (was %d)", fs.killed, fr.game[kid], before)
	}
	last := fs.msgs[len(fs.msgs)-1]
	if !strings.Contains(last, "Roblox") {
		t.Fatalf("used-up message: %q", last)
	}

	// Allowed apps are never closed.
	fr.st[kid] = AppState{Key: kid, Modes: map[string]string{"roblox": core.AppAllowed, "discord": core.AppAllowed}, Stop: StopPaused}
	fr.st[kid2] = AppState{Key: kid2, Modes: map[string]string{"roblox": core.AppAllowed}}
	fs.killed = nil
	now = now.Add(3 * time.Second)
	if n, _ := g.Tick(); n != 0 {
		t.Fatalf("allowed apps closed: %v", fs.killed)
	}
}

func TestOneClosingPerAppPerTick(t *testing.T) {
	kid := "S-1-5-21-1-2-3-1001"
	steam, helper := `C:\Program Files (x86)\Steam\steam.exe`, `C:\Program Files (x86)\Steam\bin\cef\steamwebhelper.exe`
	fs := &fakeSys{procs: []Proc{{PID: 1, Session: 2, SID: kid, Path: steam}, {PID: 2, Session: 2, SID: kid, Path: helper}, {PID: 3, Session: 2, SID: kid, Path: helper}}}
	fr := &fakeRules{st: map[string]AppState{kid: {Key: kid, Modes: map[string]string{"steam": core.AppBlocked}}}, game: map[string]int{}, warned: map[string]bool{}}
	g := &Guard{Sys: fs, List: list(t), Apps: apps(t), Rules: fr, Protected: func(s string) bool { return s == kid }}
	if n, _ := g.Tick(); n != 3 || len(fr.closed) != 1 {
		t.Fatalf("three Steam processes closed (%d) but one closing counted, got %v", n, fr.closed)
	}
}

func TestAppMessages(t *testing.T) {
	for _, why := range []string{StopBlocked, StopUsed, StopBedtime, StopHomework, StopPaused} {
		for _, l := range []string{"en", "ka", "ru", "de"} {
			m := AppMessage(l, why, "Roblox")
			if !strings.Contains(m, "Roblox") || strings.Contains(m, "%s") {
				t.Errorf("%s/%s: %q", why, l, m)
			}
		}
	}
	if !strings.Contains(GameWarnMessage("ru"), "5 минут") || !strings.Contains(GameWarnMessage("de"), "Less than 5 minutes") {
		t.Error("warning text")
	}
}
