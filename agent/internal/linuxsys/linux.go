// Package linuxsys is the Linux side of the agent: accounts from /etc/passwd,
// processes from /proc, the browsers installed, the languages, and (in the
// linux-only files) signals, desktop notices and systemd. The helpers in this
// file read below Root, so the tests run on any system.
package linuxsys

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"raisesteady-guard/agent/internal/api"
	"raisesteady-guard/agent/internal/core"
	"raisesteady-guard/agent/internal/detect"
	"raisesteady-guard/agent/internal/guard"
)

// FS reads the system files below Root ("/" in use).
type FS struct{ Root string }

func (f FS) path(p string) string { return filepath.Join(f.Root, p) }

func (f FS) exists(p string) bool {
	_, err := os.Lstat(f.path(p))
	return err == nil
}

func (f FS) lines(p string) []string {
	file, err := os.Open(f.path(p))
	if err != nil {
		return nil
	}
	defer file.Close()
	var out []string
	sc := bufio.NewScanner(file)
	sc.Buffer(make([]byte, 64<<10), 1<<20)
	for sc.Scan() {
		out = append(out, sc.Text())
	}
	return out
}

// User is one line of /etc/passwd.
type User struct {
	Name  string
	UID   int
	GID   int
	Home  string
	Shell string
}

// uidRange reads UID_MIN and UID_MAX from /etc/login.defs (1000 to 60000 by default).
func (f FS) uidRange() (int, int) {
	lo, hi := 1000, 60000
	for _, l := range f.lines("etc/login.defs") {
		fields := strings.Fields(l)
		if len(fields) != 2 {
			continue
		}
		n, err := strconv.Atoi(fields[1])
		if err != nil {
			continue
		}
		switch fields[0] {
		case "UID_MIN":
			lo = n
		case "UID_MAX":
			hi = n
		}
	}
	return lo, hi
}

// IsPersonUID reports whether a uid belongs to a person (not root, not a
// system account, not nobody).
func (f FS) IsPersonUID(uid int) bool {
	lo, hi := f.uidRange()
	return uid >= lo && uid <= hi && uid != 65534
}

// Users returns the people's accounts (uid in the login range, a login shell).
func (f FS) Users() []User {
	var out []User
	for _, l := range f.lines("etc/passwd") {
		p := strings.Split(l, ":")
		if len(p) < 7 {
			continue
		}
		uid, err1 := strconv.Atoi(p[2])
		gid, err2 := strconv.Atoi(p[3])
		if err1 != nil || err2 != nil || !f.IsPersonUID(uid) {
			continue
		}
		sh := filepath.Base(p[6])
		if sh == "nologin" || sh == "false" {
			continue
		}
		out = append(out, User{Name: p[0], UID: uid, GID: gid, Home: p[5], Shell: p[6]})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].UID < out[j].UID })
	return out
}

// UserByUID returns the account with this uid.
func (f FS) UserByUID(uid int) (User, bool) {
	for _, l := range f.lines("etc/passwd") {
		p := strings.Split(l, ":")
		if len(p) < 7 {
			continue
		}
		if n, err := strconv.Atoi(p[2]); err == nil && n == uid {
			g, _ := strconv.Atoi(p[3])
			return User{Name: p[0], UID: n, GID: g, Home: p[5], Shell: p[6]}, true
		}
	}
	return User{}, false
}

// AdminGroups are the groups whose members may use sudo (Debian and Ubuntu:
// sudo and admin; Fedora, Arch and openSUSE: wheel).
var AdminGroups = []string{"sudo", "wheel", "admin"}

// Admins returns the names of the accounts in an administrator group.
func (f FS) Admins() map[string]bool {
	out := map[string]bool{}
	gids := map[int]bool{}
	for _, l := range f.lines("etc/group") {
		p := strings.Split(l, ":")
		if len(p) < 4 {
			continue
		}
		admin := false
		for _, g := range AdminGroups {
			if p[0] == g {
				admin = true
			}
		}
		if !admin {
			continue
		}
		if gid, err := strconv.Atoi(p[2]); err == nil {
			gids[gid] = true
		}
		for _, m := range strings.Split(p[3], ",") {
			if m = strings.TrimSpace(m); m != "" {
				out[m] = true
			}
		}
	}
	for _, u := range f.Users() {
		if gids[u.GID] {
			out[u.Name] = true
		}
	}
	return out
}

// LocalAccounts lists the people's accounts; SID is the uid as text.
func (f FS) LocalAccounts() []api.LocalAccount {
	admins := f.Admins()
	var out []api.LocalAccount
	for _, u := range f.Users() {
		out = append(out, api.LocalAccount{SID: strconv.Itoa(u.UID), Name: u.Name, Admin: admins[u.Name]})
	}
	return out
}

func localeValue(lines []string, keys ...string) string {
	for _, k := range keys {
		for _, l := range lines {
			l = strings.TrimSpace(l)
			if v, ok := strings.CutPrefix(l, k+"="); ok {
				v = strings.Trim(v, `"'`)
				if v != "" {
					return v
				}
			}
		}
	}
	return ""
}

// SystemLang is the computer's language (en, ka or ru), or "".
func (f FS) SystemLang() string {
	for _, p := range []string{"etc/locale.conf", "etc/default/locale"} {
		if v := localeValue(f.lines(p), "LANG"); v != "" {
			return guard.LangFromLocale(v)
		}
	}
	return ""
}

// UserLang is an account's language from AccountsService (GNOME, KDE and
// most desktops keep it there), or "".
func (f FS) UserLang(sid string) string {
	uid, err := strconv.Atoi(sid)
	if err != nil {
		return ""
	}
	u, ok := f.UserByUID(uid)
	if !ok {
		return ""
	}
	if v := localeValue(f.lines(filepath.Join("var/lib/AccountsService/users", u.Name)), "Language", "FormatsLocale"); v != "" {
		return guard.LangFromLocale(v)
	}
	return ""
}

// LangFromEnv maps the environment's locale (LC_ALL, LC_MESSAGES, LANG) to en, ka or ru.
func LangFromEnv(getenv func(string) string) string {
	for _, k := range []string{"LC_ALL", "LC_MESSAGES", "LANG", "LANGUAGE"} {
		if v := getenv(k); v != "" && v != "C" && v != "POSIX" {
			return guard.LangFromLocale(v)
		}
	}
	return "en"
}

// Process paths ---------------------------------------------------------------

// flatpakInfo reads the app id and whether it is installed for the whole
// computer from /proc/<pid>/root/.flatpak-info.
func flatpakInfo(lines []string) (id string, system bool) {
	section := ""
	for _, l := range lines {
		l = strings.TrimSpace(l)
		if strings.HasPrefix(l, "[") {
			section = l
			continue
		}
		k, v, ok := strings.Cut(l, "=")
		if !ok {
			continue
		}
		switch {
		case section == "[Application]" && k == "name":
			id = v
		case section == "[Instance]" && k == "app-path":
			system = !strings.HasPrefix(v, "/home/") && !strings.Contains(v, "/.local/share/flatpak/")
		}
	}
	return id, system
}

// Expect is the policy files we wrote, by browser (chrome, chromium, edge,
// brave, firefox): relative path -> SHA-256 of the content.
type Expect map[string]map[string]string

// FirefoxFlatpakFiles is the program folder of the Firefox Flatpak installed
// for the whole computer.
const FirefoxFlatpakFiles = "var/lib/flatpak/app/org.mozilla.firefox/current/active/files/lib/firefox"

type rawProc struct {
	guard.Proc
	ppid uint32
}

// Processes lists the running programs with their owner (uid as SID; the
// session number is the uid too, so notices go to that person's desktop).
//
// A supported browser counts only if it can see our policy files: its main
// process (whose parent is not the same browser) either shares the
// computer's mount namespace or reads our exact file through
// /proc/<pid>/root. Otherwise (the files hidden by a private mount, for
// example) its path becomes "unverified:<path>" and it is closed. A process
// claiming to be the Firefox Flatpak must run that Flatpak's own program.
func (f FS) Processes(exp Expect) ([]guard.Proc, error) {
	ents, err := os.ReadDir(f.path("proc"))
	if err != nil {
		return nil, err
	}
	var ffFiles []os.FileInfo
	ffLoaded := false
	genuineFirefoxFlatpak := func(pidDir string) bool {
		if !ffLoaded {
			ffLoaded = true
			if des, err := os.ReadDir(f.path(FirefoxFlatpakFiles)); err == nil {
				for _, d := range des {
					if fi, err := os.Stat(filepath.Join(f.path(FirefoxFlatpakFiles), d.Name())); err == nil && fi.Mode().IsRegular() {
						ffFiles = append(ffFiles, fi)
					}
				}
			}
		}
		exe, err := os.Stat(f.path(filepath.Join(pidDir, "exe")))
		if err != nil {
			return false
		}
		for _, fi := range ffFiles {
			if os.SameFile(exe, fi) {
				return true
			}
		}
		return false
	}
	var all []rawProc
	byPID := map[uint32]*rawProc{}
	for _, e := range ents {
		pid, err := strconv.ParseUint(e.Name(), 10, 32)
		if err != nil {
			continue
		}
		dir := filepath.Join("proc", e.Name())
		uid, ppid := -1, uint64(0)
		for _, l := range f.lines(filepath.Join(dir, "status")) {
			if v, ok := strings.CutPrefix(l, "Uid:"); ok {
				if fs := strings.Fields(v); len(fs) > 0 {
					uid, _ = strconv.Atoi(fs[0])
				}
			} else if v, ok := strings.CutPrefix(l, "PPid:"); ok {
				ppid, _ = strconv.ParseUint(strings.TrimSpace(v), 10, 32)
			}
		}
		if uid < 0 {
			continue
		}
		exe, err := os.Readlink(f.path(filepath.Join(dir, "exe")))
		if err != nil {
			continue // a kernel thread, or gone
		}
		p := strings.TrimSuffix(exe, " (deleted)")
		// .flatpak-info is written by whoever made the process's root, so it is
		// only a claim: the Firefox Flatpak counts only when the program is that
		// Flatpak's own file; any other claim keeps the program path next to it
		// (a claim of a browser id closes it, a claim of anything else changes nothing).
		if info := f.lines(filepath.Join(dir, "root", ".flatpak-info")); len(info) > 0 {
			id, system := flatpakInfo(info)
			switch {
			case id == "" || strings.Contains(id, ":"):
			case system && id == "org.mozilla.firefox" && genuineFirefoxFlatpak(dir):
				p = "flatpak:" + id
			default:
				p = "flatpak-claim:" + id + ":" + p
			}
		}
		all = append(all, rawProc{guard.Proc{PID: uint32(pid), Session: uint32(uid), SID: strconv.Itoa(uid), Path: p}, uint32(ppid)})
	}
	for i := range all {
		byPID[all[i].PID] = &all[i]
	}
	hostNS, _ := os.Readlink(f.path("proc/1/ns/mnt"))
	out := make([]guard.Proc, 0, len(all))
	for i := range all {
		p := &all[i]
		if id := guard.LinuxSupportedID(p.Path); id != "" && len(exp) > 0 && !f.verified(p, byPID, hostNS, id, exp) {
			p.Path = "unverified:" + p.Path
		}
		out = append(out, p.Proc)
	}
	return out, nil
}

func (f FS) verified(p *rawProc, byPID map[uint32]*rawProc, hostNS, id string, exp Expect) bool {
	if parent := byPID[p.ppid]; parent != nil && guard.LinuxSupportedID(parent.Path) == id {
		return true // a child process of the browser (its sandbox, its tabs)
	}
	if strings.HasPrefix(p.Path, "flatpak:") {
		return true // checked above: the Flatpak's own program
	}
	dir := filepath.Join("proc", strconv.FormatUint(uint64(p.PID), 10))
	if ns, err := os.Readlink(f.path(filepath.Join(dir, "ns", "mnt"))); err == nil && hostNS != "" && ns == hostNS {
		return true // sees the computer's own files
	}
	files := exp[id]
	if len(files) == 0 {
		return true // nothing written for this browser
	}
	// Every file of this browser must be there with our content: a file that
	// cannot be read (hidden, or refused by a private mount) means "not verified".
	for rel, want := range files {
		b, err := os.ReadFile(f.path(filepath.Join(dir, "root", rel)))
		if err != nil || sum(b) != want {
			return false
		}
	}
	return true
}

func sum(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

// ExpectFrom groups the written policy files by browser.
func ExpectFrom(written map[string]string) Expect {
	exp := Expect{}
	for p, h := range written {
		id := ""
		switch {
		case strings.HasPrefix(p, "etc/opt/chrome/"):
			id = "chrome"
		case strings.HasPrefix(p, "etc/chromium/"), strings.HasPrefix(p, "etc/chromium-browser/"):
			id = "chromium"
		case strings.HasPrefix(p, "etc/opt/edge/"):
			id = "edge"
		case strings.HasPrefix(p, "etc/brave/"):
			id = "brave"
		case strings.HasPrefix(p, "etc/firefox/"), strings.HasPrefix(p, "etc/firefox-esr/"):
			id = "firefox"
		default:
			continue
		}
		if exp[id] == nil {
			exp[id] = map[string]string{}
		}
		exp[id][p] = h
	}
	return exp
}

// Installed browsers ----------------------------------------------------------

var supportedInstalls = []struct{ Path, Name, ID string }{
	{"/opt/google/chrome/chrome", "Google Chrome", "chrome"},
	{"/opt/google/chrome-beta/chrome", "Google Chrome Beta", "chrome"},
	{"/opt/google/chrome-unstable/chrome", "Google Chrome Dev", "chrome"},
	{"/usr/lib/chromium/chromium", "Chromium", "chrome"},
	{"/usr/lib64/chromium-browser/chromium-browser", "Chromium", "chrome"},
	{"/usr/lib/chromium-browser/chromium-browser", "Chromium", "chrome"},
	{"/snap/chromium/current", "Chromium (snap)", "chrome"},
	{"/opt/microsoft/msedge/msedge", "Microsoft Edge", "edge"},
	{"/opt/microsoft/msedge-beta/msedge", "Microsoft Edge Beta", "edge"},
	{"/opt/microsoft/msedge-dev/msedge", "Microsoft Edge Dev", "edge"},
	{"/opt/brave.com/brave/brave", "Brave", "brave"},
	{"/opt/brave.com/brave-beta/brave", "Brave Beta", "brave"},
	{"/opt/brave.com/brave-nightly/brave", "Brave Nightly", "brave"},
	{"/usr/lib/firefox/firefox", "Firefox", "firefox"},
	{"/usr/lib64/firefox/firefox", "Firefox", "firefox"},
	{"/usr/lib/firefox-esr/firefox-esr", "Firefox ESR", "firefox"},
	{"/opt/firefox/firefox", "Firefox", "firefox"},
	{"/snap/firefox/current", "Firefox (snap)", "firefox"},
	{"/var/lib/flatpak/app/org.mozilla.firefox", "Firefox (Flatpak)", "firefox"},
}

// Program names that are often wrapper scripts of a supported browser in
// /usr/bin: they are not scanned there.
func wrapperName(list core.UnsupportedList, name string) bool {
	for _, b := range list.Browsers {
		if b.ID == "chromium" || strings.HasSuffix(b.ID, "-other") {
			for _, e := range b.LinuxExe {
				if strings.EqualFold(e, name) {
					return true
				}
			}
		}
	}
	return false
}

// DetectBrowsers lists the supported and unsupported browsers installed: the
// supported ones in their usual folders, unsupported Flatpaks (for the whole
// computer or in a home folder), snaps, and programs in /usr/bin.
func (f FS) DetectBrowsers(list core.UnsupportedList) []detect.Found {
	var out []detect.Found
	seen := map[string]bool{}
	add := func(fd detect.Found) {
		if seen[fd.Path] {
			return
		}
		seen[fd.Path] = true
		out = append(out, fd)
	}
	for _, s := range supportedInstalls {
		if f.exists(s.Path) {
			add(detect.Found{ID: s.ID, Name: s.Name, Path: s.Path, Supported: true})
		}
	}
	unsupported := func(path, sid string, m guard.Match) {
		if m.Unsupported != nil {
			add(detect.Found{ID: m.Unsupported.ID, Name: m.Unsupported.Name, Path: path, ProfileSID: sid})
		}
	}
	homes := map[string]string{}
	for _, u := range f.Users() {
		homes[strconv.Itoa(u.UID)] = u.Home
	}
	sids := make([]string, 0, len(homes))
	for s := range homes {
		sids = append(sids, s)
	}
	sort.Strings(sids)
	for _, b := range list.Browsers {
		for _, id := range b.Flatpak {
			if p := "/var/lib/flatpak/app/" + id; f.exists(p) && guard.LinuxSupportedID("flatpak:"+id) == "" {
				unsupported(p, "", guard.ClassifyLinux(list, "flatpak:"+id, ""))
			}
			for _, sid := range sids {
				if p := filepath.Join(homes[sid], ".local/share/flatpak/app", id); f.exists(p) {
					unsupported(p, sid, guard.ClassifyLinux(list, "flatpak-user:"+id, ""))
				}
			}
		}
		for _, s := range b.Snap {
			if p := "/snap/" + s + "/current"; f.exists(p) {
				unsupported("/snap/"+s, "", guard.ClassifyLinux(list, "/snap/"+s+"/x", ""))
			}
		}
		for _, e := range b.LinuxExe {
			if wrapperName(list, e) {
				continue
			}
			for _, dir := range []string{"/usr/bin", "/usr/local/bin"} {
				p := dir + "/" + e
				if !f.exists(p) {
					continue
				}
				target := p
				if t, err := filepath.EvalSymlinks(f.path(p)); err == nil {
					realRoot, err := filepath.EvalSymlinks(f.Root)
					if err != nil {
						realRoot = f.Root
					}
					if rel, err := filepath.Rel(realRoot, t); err == nil {
						target = "/" + filepath.ToSlash(rel)
					}
				}
				if guard.LinuxSupportedID(target) != "" {
					continue
				}
				unsupported(target, "", guard.ClassifyLinux(list, p, ""))
			}
		}
	}
	return out
}

// Unit is the systemd unit name.
const Unit = "raisesteady-guard.service"

// UnitText is the unit install.sh and the packages install (also written by
// Install when none is found).
func UnitText(exe string) string {
	return `# RaiseSteady Guard: safe browser settings for children.
[Unit]
Description=RaiseSteady Guard: safe browser settings for children
Documentation=https://github.com/tengotengo23/raisesteady-guard
After=local-fs.target
ConditionPathExists=/var/lib/raisesteady-guard/config.json

[Service]
Type=simple
ExecStart=` + exe + ` service
Restart=always
RestartSec=5
ProtectSystem=true
ProtectHome=read-only
PrivateTmp=true
UMask=0077

[Install]
WantedBy=multi-user.target
`
}
