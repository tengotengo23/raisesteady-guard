package linuxsys

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"raisesteady-guard/agent/internal/core"
)

// Opening the parent page with no terminal (P2-194). Two sides:
//
//   - root (the package's post-install step) finds the person at the screen
//     and starts the opener in that person's own desktop session;
//   - the opener (`raisesteady-guard open-page`, an ordinary user program,
//     also behind the app-menu shortcut and the sign-in entry) asks the
//     service whether it runs and opens the page in the default browser.
//
// The parts that only read files or decide are here, so they are tested on
// any system; the parts that start programs are in sys_linux.go.

// ParentURL is the parent page on this computer.
func ParentURL() string { return fmt.Sprintf("http://127.0.0.1:%d/parent", core.DefaultPort) }

// Session is what `loginctl show-session` says about one login session.
type Session struct {
	UID    int
	Type   string // wayland, x11, mir, tty, unspecified
	Class  string // user, greeter, lock-screen, background, manager
	Active bool
	Remote bool
}

// ParseSession reads the Key=Value lines of `loginctl show-session ID
// -p User -p Type -p Class -p Active -p Remote`.
func ParseSession(out string) (Session, bool) {
	s := Session{UID: -1}
	for _, l := range strings.Split(out, "\n") {
		k, v, ok := strings.Cut(strings.TrimSpace(l), "=")
		if !ok {
			continue
		}
		switch k {
		case "User":
			if n, err := strconv.Atoi(v); err == nil {
				s.UID = n
			}
		case "Type":
			s.Type = v
		case "Class":
			s.Class = v
		case "Active":
			s.Active = v == "yes"
		case "Remote":
			s.Remote = v == "yes"
		}
	}
	return s, s.UID >= 0
}

// AtScreen reports whether a person sits at this session's screen now: a
// desktop session (not the sign-in screen, not ssh) that is in front.
func (s Session) AtScreen() bool {
	graphical := s.Type == "wayland" || s.Type == "x11" || s.Type == "mir"
	return graphical && s.Class == "user" && s.Active && !s.Remote
}

// DesktopUIDs picks the people to open the parent page for: each person
// whose desktop session is in front, once.
func DesktopUIDs(list []Session, isPerson func(int) bool) []int {
	var out []int
	seen := map[int]bool{}
	for _, s := range list {
		if !s.AtScreen() || seen[s.UID] || !isPerson(s.UID) {
			continue
		}
		seen[s.UID] = true
		out = append(out, s.UID)
	}
	return out
}

// OpenUnitArgs are the arguments of systemd-run that start the opener in a
// person's own session. A unit of the user's service manager has the desktop
// session's environment (WAYLAND_DISPLAY, DISPLAY and the rest), which a
// program started by root does not. KillMode=process: the browser that the
// opener starts must stay when the opener itself ends.
func OpenUnitArgs(exe string) []string {
	return []string{"--user", "--collect", "--quiet", "--no-block",
		"-p", "KillMode=process",
		"--description=" + core.AppName + ": open the parent page",
		exe, "open-page", "--wait"}
}

// PortalArgs are the gdbus arguments that ask the desktop portal to open
// an address in the default browser. It needs the session bus only.
func PortalArgs(url string) []string {
	return []string{"call", "--session", "--timeout", "10",
		"--dest", "org.freedesktop.portal.Desktop",
		"--object-path", "/org/freedesktop/portal/desktop",
		"--method", "org.freedesktop.portal.OpenURI.OpenURI", "", url, "{}"}
}

// shortcutFile is our app-menu entry's name; a file of the same name in a
// person's own folder replaces the system one.
const shortcutFile = "raisesteady-guard.desktop"

// hidesEntry reports whether a desktop file hides its entry from the menu.
func hidesEntry(text string) bool {
	in := false
	for _, l := range strings.Split(text, "\n") {
		l = strings.TrimSpace(l)
		if strings.HasPrefix(l, "[") {
			in = l == "[Desktop Entry]"
			continue
		}
		if !in {
			continue
		}
		k, v, ok := strings.Cut(l, "=")
		if !ok {
			continue
		}
		k, v = strings.TrimSpace(k), strings.ToLower(strings.TrimSpace(v))
		if (k == "Hidden" || k == "NoDisplay") && v == "true" {
			return true
		}
	}
	return false
}

// FixShortcut keeps RaiseSteady Guard in the app menu: a copy of our menu
// entry in the person's own folder that hides it (found on a real PC,
// 10 Nov 2026) is moved aside, so the system entry shows again. It returns
// the new name of a moved file. Nothing else is touched.
func FixShortcut(home string) (string, error) {
	if home == "" {
		return "", nil
	}
	p := filepath.Join(home, ".local", "share", "applications", shortcutFile)
	st, err := os.Lstat(p)
	if err != nil || !st.Mode().IsRegular() || st.Size() > 64<<10 {
		return "", nil
	}
	b, err := os.ReadFile(p)
	if err != nil || !hidesEntry(string(b)) {
		return "", nil
	}
	aside := p + ".hidden"
	if err := os.Rename(p, aside); err != nil {
		return "", err
	}
	return aside, nil
}

var openText = map[string]map[string]string{
	"notRunning": {
		"en": "RaiseSteady Guard is not running. Restart the computer, then open it again.",
		"ka": "RaiseSteady Guard არ მუშაობს. გადატვირთეთ კომპიუტერი და გახსენით თავიდან.",
		"ru": "RaiseSteady Guard не запущен. Перезагрузите компьютер и откройте его снова.",
	},
	"address": {
		"en": "Open this address in your browser: %s",
		"ka": "გახსენით ეს მისამართი ბრაუზერში: %s",
		"ru": "Откройте этот адрес в браузере: %s",
	},
}

func ot(lang, key string) string {
	if s := openText[key][lang]; s != "" {
		return s
	}
	return openText[key]["en"]
}

// Opener opens the parent page for the person who runs it.
type Opener struct {
	FS   FS
	UID  int
	Home string
	Lang string
	Env  func(string) string
	// State asks the service: does the first-run setup still wait?
	State func() (setupPending bool, err error)
	// Run starts a program and waits for it; an error when the program is
	// missing or fails.
	Run    func(name string, args ...string) error
	Notify func(title, text string)
	Sleep  func(time.Duration)
	// Tries is how many times --wait asks the service (two a second).
	Tries int
}

// ErrNotRunning: the service did not answer.
var ErrNotRunning = errors.New("the RaiseSteady Guard service is not running")

// Open opens the parent page.
//
// wait: the service was started a moment ago; ask until it answers.
// login: called at sign-in. It keeps the menu shortcut visible, and opens
// the page only while the first-run setup waits and only for an
// administrator (a child's sign-in must not be handed the setup).
func (o *Opener) Open(wait, login bool) error {
	if login {
		_, _ = FixShortcut(o.Home)
	}
	tries := 1
	if wait || login {
		tries = o.Tries
	}
	var pending bool
	var err error
	for i := 0; i < tries; i++ {
		if pending, err = o.State(); err == nil {
			break
		}
		if i+1 < tries {
			o.Sleep(500 * time.Millisecond)
		}
	}
	if err != nil {
		if login {
			return nil
		}
		o.Notify(core.AppName, ot(o.Lang, "notRunning"))
		return ErrNotRunning
	}
	if login {
		if !pending {
			return nil
		}
		if admins := o.FS.Admins(); len(admins) > 0 {
			if u, ok := o.FS.UserByUID(o.UID); !ok || !admins[u.Name] {
				return nil
			}
		}
	}
	url := ParentURL()
	// xdg-open needs the desktop's environment; the portal needs the session
	// bus only, so it also works where the environment is bare.
	if o.Env("WAYLAND_DISPLAY") != "" || o.Env("DISPLAY") != "" {
		if o.Run("xdg-open", url) == nil || o.Run("gio", "open", url) == nil {
			return nil
		}
	}
	if o.Run("gdbus", PortalArgs(url)...) == nil {
		return nil
	}
	o.Notify(core.AppName, fmt.Sprintf(ot(o.Lang, "address"), url))
	return errors.New("no way to open a browser from here")
}
