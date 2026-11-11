//go:build !windows

package linuxsys

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"raisesteady-guard/agent/internal/guard"
)

// Opening the parent page with no terminal, and Yandex Browser (P2-194).

func TestParseSessionAndWhoIsAtTheScreen(t *testing.T) {
	cases := []struct {
		name, out string
		ok, at    bool
		uid       int
	}{
		{"GNOME on Wayland", "User=1000\nType=wayland\nClass=user\nActive=yes\nRemote=no\n", true, true, 1000},
		{"X11", "Type=x11\nUser=1001\nClass=user\nActive=yes\nRemote=no", true, true, 1001},
		{"the sign-in screen", "User=120\nType=wayland\nClass=greeter\nActive=yes\nRemote=no\n", true, false, 120},
		{"switched away", "User=1000\nType=wayland\nClass=user\nActive=no\nRemote=no\n", true, false, 1000},
		{"ssh", "User=1000\nType=tty\nClass=user\nActive=yes\nRemote=yes\n", true, false, 1000},
		{"a remote desktop", "User=1000\nType=x11\nClass=user\nActive=yes\nRemote=yes\n", true, false, 1000},
		{"a text console", "User=1000\nType=tty\nClass=user\nActive=yes\nRemote=no\n", true, false, 1000},
		{"the user's service manager", "User=1000\nType=unspecified\nClass=manager\nActive=yes\nRemote=no\n", true, false, 1000},
		{"nothing", "", false, false, -1},
		{"no user", "Type=wayland\nClass=user\nActive=yes\n", false, false, -1},
		{"a broken number", "User=abc\nType=wayland\nClass=user\nActive=yes\n", false, false, -1},
	}
	for _, c := range cases {
		s, ok := ParseSession(c.out)
		if ok != c.ok || s.UID != c.uid || (ok && s.AtScreen() != c.at) {
			t.Errorf("%s: session %+v ok=%v at=%v", c.name, s, ok, s.AtScreen())
		}
	}
}

func TestDesktopUIDs(t *testing.T) {
	at := func(uid int) Session {
		return Session{UID: uid, Type: "wayland", Class: "user", Active: true}
	}
	list := []Session{
		{UID: 120, Type: "wayland", Class: "greeter", Active: true},
		at(1000), at(1000), // two sessions of one person: once
		{UID: 1001, Type: "wayland", Class: "user"}, // not in front
		at(0),   // root's desktop: not a person
		at(998), // a service account
		at(1002),
	}
	person := func(uid int) bool { return uid >= 1000 && uid < 60000 }
	got := DesktopUIDs(list, person)
	if len(got) != 2 || got[0] != 1000 || got[1] != 1002 {
		t.Errorf("DesktopUIDs = %v, want [1000 1002]", got)
	}
	if got := DesktopUIDs(nil, person); len(got) != 0 {
		t.Errorf("no sessions: %v", got)
	}
}

func TestOpenUnitAndPortalArguments(t *testing.T) {
	const exe = "/usr/lib/raisesteady-guard/raisesteady-guard"
	a := OpenUnitArgs(exe)
	joined := strings.Join(a, " ")
	for _, need := range []string{"--user", "--collect", "--no-block", "-p KillMode=process"} {
		if !strings.Contains(joined, need) {
			t.Errorf("systemd-run arguments lack %q: %v", need, a)
		}
	}
	if n := len(a); n < 3 || a[n-3] != exe || a[n-2] != "open-page" || a[n-1] != "--wait" {
		t.Errorf("the unit must run `%s open-page --wait`: %v", exe, a)
	}
	for _, x := range a {
		if strings.Contains(x, "--uid") || strings.Contains(x, "--machine") || strings.Contains(x, "sh") && x != exe && !strings.Contains(x, "--description") {
			t.Errorf("unexpected argument %q", x)
		}
	}
	p := PortalArgs("http://127.0.0.1:47615/parent")
	pj := strings.Join(p, " ")
	if !strings.Contains(pj, "--session") || !strings.Contains(pj, "--dest org.freedesktop.portal.Desktop") ||
		!strings.Contains(pj, "--method org.freedesktop.portal.OpenURI.OpenURI") || p[len(p)-2] != "http://127.0.0.1:47615/parent" {
		t.Errorf("portal arguments: %v", p)
	}
	if ParentURL() != "http://127.0.0.1:47615/parent" {
		t.Errorf("ParentURL = %s", ParentURL())
	}
}

func TestHidesEntry(t *testing.T) {
	cases := map[string]bool{
		"[Desktop Entry]\nName=RaiseSteady Guard\nHidden=true\n":                true,
		"[Desktop Entry]\nName=RaiseSteady Guard\nNoDisplay=true\n":             true,
		"[Desktop Entry]\r\nHidden = True \r\n":                                 true,
		"[Desktop Entry]\nName=RaiseSteady Guard\nHidden=false\n":               false,
		"[Desktop Entry]\nName=RaiseSteady Guard\nExec=x\n":                     false,
		"[Desktop Entry]\nName=x\n[Desktop Action a]\nHidden=true\n":            false,
		"Hidden=true\n[Desktop Entry]\nName=x\n":                                false,
		"[Desktop Entry]\n# Hidden=true\nName[ka]=Hidden=true\nX-Hidden=true\n": false,
		"": false,
	}
	for text, want := range cases {
		if got := hidesEntry(text); got != want {
			t.Errorf("hidesEntry(%q) = %v, want %v", text, got, want)
		}
	}
}

// Bug 4: a copy of our menu entry in a person's own folder with Hidden=true
// took RaiseSteady Guard out of the app menu. The copy is moved aside; a
// person's own visible copy, and every other file, stay.
func TestFixShortcut(t *testing.T) {
	home := t.TempDir()
	apps := filepath.Join(home, ".local/share/applications")
	ours := filepath.Join(apps, "raisesteady-guard.desktop")
	hidden := "[Desktop Entry]\nType=Application\nName=RaiseSteady Guard\nHidden=true\n"

	if moved, err := FixShortcut(home); moved != "" || err != nil {
		t.Fatalf("no file: %q %v", moved, err)
	}
	if moved, err := FixShortcut(""); moved != "" || err != nil {
		t.Fatalf("no home: %q %v", moved, err)
	}
	write(t, home, ".local/share/applications/raisesteady-guard.desktop", hidden)
	write(t, home, ".local/share/applications/other.desktop", hidden)
	moved, err := FixShortcut(home)
	if err != nil || moved != ours+".hidden" {
		t.Fatalf("moved %q, err %v", moved, err)
	}
	if _, err := os.Stat(ours); !errors.Is(err, os.ErrNotExist) {
		t.Error("the hiding copy is still there")
	}
	if b, _ := os.ReadFile(ours + ".hidden"); string(b) != hidden {
		t.Error("the copy must be kept under its new name")
	}
	if _, err := os.Stat(filepath.Join(apps, "other.desktop")); err != nil {
		t.Error("another program's file was touched")
	}
	// A second hidden copy replaces the first kept one; still no error.
	write(t, home, ".local/share/applications/raisesteady-guard.desktop", hidden)
	if moved, err := FixShortcut(home); err != nil || moved == "" {
		t.Fatalf("second time: %q %v", moved, err)
	}

	// The person's own visible copy (say, with another icon) is theirs.
	visible := "[Desktop Entry]\nType=Application\nName=Guard\nExec=/usr/lib/raisesteady-guard/open-parent-page\n"
	write(t, home, ".local/share/applications/raisesteady-guard.desktop", visible)
	if moved, _ := FixShortcut(home); moved != "" {
		t.Error("a visible copy was moved")
	}
	// A link is never followed or moved.
	if err := os.Remove(ours); err != nil {
		t.Fatal(err)
	}
	write(t, home, "elsewhere.desktop", hidden)
	if err := os.Symlink(filepath.Join(home, "elsewhere.desktop"), ours); err != nil {
		t.Fatal(err)
	}
	if moved, _ := FixShortcut(home); moved != "" {
		t.Error("a link was moved")
	}
	if _, err := os.Lstat(ours); err != nil {
		t.Error("the link is gone")
	}
}

// fakeDesk records what an Opener does.
type fakeDesk struct {
	stateErrs int // State fails this many times first
	pending   bool
	fail      map[string]bool // programs that fail
	ran       []string
	notes     []string
	slept     int
	asked     int
}

func (d *fakeDesk) opener(root string, uid int, env map[string]string) *Opener {
	return &Opener{
		FS: FS{Root: root}, UID: uid, Lang: "en",
		Env: func(k string) string { return env[k] },
		State: func() (bool, error) {
			d.asked++
			if d.asked <= d.stateErrs {
				return false, errors.New("connection refused")
			}
			return d.pending, nil
		},
		Run: func(name string, args ...string) error {
			d.ran = append(d.ran, name+" "+strings.Join(args, " "))
			if d.fail[name] {
				return errors.New(name + " failed")
			}
			return nil
		},
		Notify: func(_, text string) { d.notes = append(d.notes, text) },
		Sleep:  func(time.Duration) { d.slept++ },
		Tries:  60,
	}
}

var wayland = map[string]string{"WAYLAND_DISPLAY": "wayland-0"}

func TestOpenerOpensThePage(t *testing.T) {
	const url = "http://127.0.0.1:47615/parent"
	root := t.TempDir()

	d := &fakeDesk{}
	if err := d.opener(root, 1000, wayland).Open(false, false); err != nil {
		t.Fatal(err)
	}
	if len(d.ran) != 1 || d.ran[0] != "xdg-open "+url || len(d.notes) != 0 || d.asked != 1 {
		t.Errorf("ran %v, notes %v, asked %d", d.ran, d.notes, d.asked)
	}

	// X11 counts as a desktop too.
	d = &fakeDesk{}
	_ = d.opener(root, 1000, map[string]string{"DISPLAY": ":0"}).Open(false, false)
	if len(d.ran) != 1 || !strings.HasPrefix(d.ran[0], "xdg-open ") {
		t.Errorf("X11: ran %v", d.ran)
	}

	// No xdg-open on this PC: gio opens it.
	d = &fakeDesk{fail: map[string]bool{"xdg-open": true}}
	if err := d.opener(root, 1000, wayland).Open(false, false); err != nil {
		t.Fatal(err)
	}
	if len(d.ran) != 2 || d.ran[1] != "gio open "+url {
		t.Errorf("ran %v", d.ran)
	}

	// Started by root with a bare environment: only the session bus is
	// there, so the desktop portal is asked and xdg-open is not tried.
	d = &fakeDesk{}
	if err := d.opener(root, 1000, nil).Open(false, false); err != nil {
		t.Fatal(err)
	}
	if len(d.ran) != 1 || !strings.HasPrefix(d.ran[0], "gdbus call --session") || !strings.Contains(d.ran[0], "OpenURI.OpenURI") || !strings.Contains(d.ran[0], url) {
		t.Errorf("ran %v", d.ran)
	}

	// Nothing works: the person is told the address, under no other text.
	d = &fakeDesk{fail: map[string]bool{"xdg-open": true, "gio": true, "gdbus": true}}
	if err := d.opener(root, 1000, wayland).Open(false, false); err == nil {
		t.Error("no error when nothing could open the page")
	}
	if len(d.ran) != 3 || len(d.notes) != 1 || !strings.Contains(d.notes[0], url) {
		t.Errorf("ran %v, notes %v", d.ran, d.notes)
	}
}

func TestOpenerWhenTheServiceDoesNotAnswer(t *testing.T) {
	root := t.TempDir()
	// From the app menu: one question, then a plain notice and no browser.
	d := &fakeDesk{stateErrs: 1000}
	err := d.opener(root, 1000, wayland).Open(false, false)
	if !errors.Is(err, ErrNotRunning) || d.asked != 1 || d.slept != 0 || len(d.ran) != 0 || len(d.notes) != 1 {
		t.Fatalf("err %v, asked %d, slept %d, ran %v, notes %v", err, d.asked, d.slept, d.ran, d.notes)
	}
	if strings.Contains(d.notes[0], "sudo") || strings.Contains(strings.ToLower(d.notes[0]), "terminal") {
		t.Errorf("the notice must not send a parent to a terminal: %q", d.notes[0])
	}
	for lang, word := range map[string]string{"ka": "არ მუშაობს", "ru": "не запущен", "de": "is not running"} {
		d := &fakeDesk{stateErrs: 1000}
		o := d.opener(root, 1000, wayland)
		o.Lang = lang
		_ = o.Open(false, false)
		if len(d.notes) != 1 || !strings.Contains(d.notes[0], word) {
			t.Errorf("%s notice = %v", lang, d.notes)
		}
	}

	// Right after the install the service needs a moment: --wait asks again.
	d = &fakeDesk{stateErrs: 3}
	if err := d.opener(root, 1000, wayland).Open(true, false); err != nil {
		t.Fatal(err)
	}
	if d.asked != 4 || d.slept != 3 || len(d.ran) != 1 {
		t.Errorf("asked %d, slept %d, ran %v", d.asked, d.slept, d.ran)
	}
	// It gives up after Tries questions.
	d = &fakeDesk{stateErrs: 1000}
	if err := d.opener(root, 1000, wayland).Open(true, false); !errors.Is(err, ErrNotRunning) || d.asked != 60 || d.slept != 59 {
		t.Errorf("err %v, asked %d, slept %d", err, d.asked, d.slept)
	}
	// At sign-in a missing service is nobody's business: no notice.
	d = &fakeDesk{stateErrs: 1000}
	if err := d.opener(root, 1000, wayland).Open(false, true); err != nil || len(d.notes) != 0 || len(d.ran) != 0 {
		t.Errorf("sign-in: err %v, notes %v, ran %v", err, d.notes, d.ran)
	}
}

// At sign-in the page opens only while the first-run setup waits, and only
// for an administrator: a child's sign-in is never handed the setup.
func TestOpenerAtSignIn(t *testing.T) {
	root := t.TempDir()
	write(t, root, "etc/passwd", passwd)
	write(t, root, "etc/group", "root:x:0:\nsudo:x:27:mum\nkid:x:1001:\n")

	run := func(uid int, pending bool) *fakeDesk {
		d := &fakeDesk{pending: pending}
		if err := d.opener(root, uid, wayland).Open(false, true); err != nil {
			t.Fatal(err)
		}
		return d
	}
	if d := run(1000, true); len(d.ran) != 1 {
		t.Errorf("the parent while setup waits: ran %v", d.ran)
	}
	if d := run(1001, true); len(d.ran) != 0 || len(d.notes) != 0 {
		t.Errorf("the child while setup waits: ran %v, notes %v", d.ran, d.notes)
	}
	if d := run(4242, true); len(d.ran) != 0 {
		t.Errorf("an unknown account: ran %v", d.ran)
	}
	if d := run(1000, false); len(d.ran) != 0 {
		t.Errorf("after the setup nothing opens at sign-in: ran %v", d.ran)
	}
	// From the app menu the page always opens, for anyone (the PIN guards it).
	d := &fakeDesk{}
	if err := d.opener(root, 1001, wayland).Open(false, false); err != nil || len(d.ran) != 1 {
		t.Errorf("app menu: err %v, ran %v", err, d.ran)
	}

	// A PC with no administrator group at all: whoever signs in gets it.
	bare := t.TempDir()
	write(t, bare, "etc/passwd", passwd)
	write(t, bare, "etc/group", "root:x:0:\n")
	d = &fakeDesk{pending: true}
	if err := d.opener(bare, 1001, wayland).Open(false, true); err != nil || len(d.ran) != 1 {
		t.Errorf("no administrators: err %v, ran %v", err, d.ran)
	}

	// Sign-in also puts the menu shortcut back (bug 4), whatever the state.
	home := t.TempDir()
	write(t, home, ".local/share/applications/raisesteady-guard.desktop", "[Desktop Entry]\nHidden=true\n")
	d = &fakeDesk{stateErrs: 1000}
	o := d.opener(root, 1001, wayland)
	o.Home, o.Tries = home, 2
	if err := o.Open(false, true); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(home, ".local/share/applications/raisesteady-guard.desktop")); !errors.Is(err, os.ErrNotExist) {
		t.Error("the hiding copy stayed after sign-in")
	}
	// The app menu click does not touch the person's files.
	write(t, home, ".local/share/applications/raisesteady-guard.desktop", "[Desktop Entry]\nHidden=true\n")
	d = &fakeDesk{}
	o = d.opener(root, 1001, wayland)
	o.Home = home
	_ = o.Open(false, false)
	if _, err := os.Stat(filepath.Join(home, ".local/share/applications/raisesteady-guard.desktop")); err != nil {
		t.Error("the app menu click moved a file")
	}
}

// Yandex Browser reads Chromium managed policies from its own folder, so it
// is protected like Chrome and no longer closed (WhatsApp runs in it as a
// web app on the PC the bugs were found on).
func TestYandexBrowserIsProtectedNotClosed(t *testing.T) {
	root := t.TempDir()
	const host = "mnt:[4026531841]"
	const file = "etc/opt/yandex/browser/policies/managed/raisesteady-guard.json"
	ours := `{"IncognitoModeAvailability": 1}`

	exp := ExpectFrom(map[string]string{file: sum([]byte(ours)), "etc/opt/chrome/policies/managed/raisesteady-guard.json": "x"})
	if len(exp) != 2 || exp["yandex"][file] != sum([]byte(ours)) {
		t.Fatalf("ExpectFrom = %v", exp)
	}

	write(t, root, "etc/passwd", passwd)
	write(t, root, "opt/yandex/browser/yandex_browser", "")
	write(t, root, "opt/yandex/browser-beta/yandex_browser-beta", "")
	list := data(t).Unsupported
	var sup, uns []string
	for _, f := range (FS{Root: root}).DetectBrowsers(list) {
		if f.Supported {
			sup = append(sup, f.Name+"="+f.ID)
		} else {
			uns = append(uns, f.ID+"@"+f.Path)
		}
	}
	if strings.Join(sup, ",") != "Yandex Browser=yandex" {
		t.Errorf("supported = %v", sup)
	}
	if len(uns) != 0 {
		t.Errorf("unsupported = %v", uns)
	}

	proc(t, root, 1, 0, 0, "/usr/lib/systemd/systemd", host)
	proc(t, root, 100, 1, 1001, "/usr/bin/bash", host)
	proc(t, root, 101, 100, 1001, "/opt/yandex/browser/yandex_browser", host)               // sees our file
	proc(t, root, 102, 101, 1001, "/opt/yandex/browser/yandex_browser", "mnt:[4026532000]") // its sandboxed child
	proc(t, root, 200, 100, 1001, "/opt/yandex/browser/yandex_browser", "mnt:[4026532100]") // started with the policies hidden
	proc(t, root, 300, 100, 1001, "/opt/yandex/browser-beta/yandex_browser-beta", host)
	proc(t, root, 400, 100, 1001, "/home/kid/y/yandex_browser", host)
	procs, err := FS{Root: root}.Processes(exp)
	if err != nil {
		t.Fatal(err)
	}
	closed := map[uint32]string{}
	open := map[uint32]string{}
	for _, p := range procs {
		m := guard.ClassifyLinux(list, p.Path, "")
		if m.Unsupported != nil {
			closed[p.PID] = m.Unsupported.ID
		}
		if m.SupportedID != "" {
			open[p.PID] = m.SupportedID
		}
	}
	if open[101] != "yandex" || open[102] != "yandex" {
		t.Errorf("Yandex Browser with our policies must stay open: %v (closed %v)", open, closed)
	}
	for _, pid := range []uint32{200, 300, 400} {
		if closed[pid] != "yandex" {
			t.Errorf("pid %d: closed as %q, want yandex", pid, closed[pid])
		}
	}
	if len(closed) != 3 {
		t.Errorf("closed = %v", closed)
	}
}
