package guard

import (
	"strings"
	"testing"
	"time"

	"raisesteady-guard/agent/internal/core"
)

func list(t *testing.T) core.UnsupportedList {
	t.Helper()
	d, err := core.LoadData()
	if err != nil {
		t.Fatal(err)
	}
	return d.Unsupported
}

func TestClassify(t *testing.T) {
	l := list(t)
	for _, c := range []struct {
		path, product, supported, unsupported string
	}{
		{`C:\Program Files\Google\Chrome\Application\chrome.exe`, "Google Chrome", "chrome", ""},
		{`C:\Program Files (x86)\Microsoft\Edge\Application\msedge.exe`, "Microsoft Edge", "edge", ""},
		{`C:\Program Files\Mozilla Firefox\firefox.exe`, "Firefox", "firefox", ""},
		{`C:\Users\kid\AppData\Local\BraveSoftware\Brave-Browser\Application\brave.exe`, "Brave Browser", "brave", ""},
		{`C:\Users\kid\AppData\Local\Programs\Opera\opera.exe`, "Opera Internet Browser", "", "opera"},
		{`C:\Users\kid\AppData\Local\Vivaldi\Application\vivaldi.exe`, "Vivaldi", "", "vivaldi"},
		{`C:\Users\kid\AppData\Local\Programs\Opera GX\opera.exe`, "Opera GX Internet Browser", "", "opera-gx"},
		{`C:\Users\kid\Desktop\Tor Browser\Browser\firefox.exe`, "Firefox", "", "tor"},
		{`C:\Users\kid\AppData\Local\Chromium\Application\chrome.exe`, "Chromium", "", "chromium"},
		{`C:\Users\kid\AppData\Local\Yandex\YandexBrowser\Application\browser.exe`, "Yandex", "", "yandex"},
		{`D:\portable\x\renamed.exe`, "Opera Internet Browser", "", "opera"},
		{`C:\Games\browser.exe`, "Some Game", "", ""},
		{`C:\Windows\explorer.exe`, "Microsoft® Windows® Operating System", "", ""},
		{`C:\Program Files (x86)\Microsoft\EdgeWebView\Application\msedgewebview2.exe`, "Microsoft Edge WebView2", "", ""},
	} {
		m := Classify(l, c.path, c.product)
		got := ""
		if m.Unsupported != nil {
			got = m.Unsupported.ID
		}
		if m.SupportedID != c.supported || got != c.unsupported {
			t.Errorf("%s (%s): supported=%q unsupported=%q, want %q %q", c.path, c.product, m.SupportedID, got, c.supported, c.unsupported)
		}
	}
}

type fakeSys struct {
	procs  []Proc
	killed []uint32
	msgs   []string
}

func (f *fakeSys) Processes() ([]Proc, error) { return f.procs, nil }
func (f *fakeSys) ProductName(path string) string {
	if strings.HasSuffix(strings.ToLower(path), "opera.exe") {
		return "Opera Internet Browser"
	}
	return "Google Chrome"
}
func (f *fakeSys) Kill(pid uint32) error { f.killed = append(f.killed, pid); return nil }
func (f *fakeSys) Message(s uint32, title, text string) error {
	f.msgs = append(f.msgs, text)
	return nil
}
func (f *fakeSys) UserLang(sid string) string { return "ka" }

func TestTickClosesOnlyInProtectedSessions(t *testing.T) {
	kid, parent := "S-1-5-21-1-2-3-1001", "S-1-5-21-1-2-3-1000"
	now := time.Date(2026, 11, 3, 12, 0, 0, 0, time.UTC)
	fs := &fakeSys{procs: []Proc{
		{PID: 10, Session: 2, SID: kid, Path: `C:\Users\kid\AppData\Local\Programs\Opera\opera.exe`},
		{PID: 11, Session: 2, SID: kid, Path: `C:\Users\kid\AppData\Local\Programs\Opera\opera.exe`},
		{PID: 12, Session: 2, SID: kid, Path: `C:\Program Files\Google\Chrome\Application\chrome.exe`},
		{PID: 20, Session: 1, SID: parent, Path: `C:\Users\parent\AppData\Local\Programs\Opera\opera.exe`},
		{PID: 30, Session: 0, SID: kid, Path: `C:\x\opera.exe`},
	}}
	g := &Guard{Sys: fs, List: list(t), Protected: func(s string) bool { return s == kid }, Now: func() time.Time { return now }}
	n, err := g.Tick()
	if err != nil || n != 2 || len(fs.killed) != 2 || fs.killed[0] != 10 || fs.killed[1] != 11 {
		t.Fatalf("closed %d: %v", n, fs.killed)
	}
	if len(fs.msgs) != 1 || !strings.Contains(fs.msgs[0], "Opera") || !strings.Contains(fs.msgs[0], "ბავშვებისთვის") {
		t.Fatalf("one Georgian message expected: %v", fs.msgs)
	}
	g.Tick() // within a minute: closed again, no second message
	if len(fs.msgs) != 1 {
		t.Fatal("message repeated within a minute")
	}
	now = now.Add(2 * time.Minute)
	g.Tick()
	if len(fs.msgs) != 2 {
		t.Fatal("message should show again after a minute")
	}
	if c := g.ClosedList(); len(c) != 1 || c[0].Count != 6 {
		t.Fatalf("closed list: %+v", c)
	}
}

func TestMessagesAndLang(t *testing.T) {
	for _, l := range []string{"en", "ka", "ru", "de"} {
		m := ClosedMessage(l, "Opera")
		if !strings.HasPrefix(m, "Opera") || strings.Contains(m, "%s") {
			t.Errorf("%s: %q", l, m)
		}
	}
	if LangFromLocale("ka-GE") != "ka" || LangFromLocale("ru-RU") != "ru" || LangFromLocale("en-GB") != "en" || LangFromLocale("") != "en" {
		t.Error("LangFromLocale")
	}
}
