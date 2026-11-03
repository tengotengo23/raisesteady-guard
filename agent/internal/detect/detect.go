// Package detect finds the browsers installed on this PC, machine-wide and
// in each user's profile, and classifies them as supported or not.
package detect

import (
	"sort"
	"strings"

	"raisesteady-guard/agent/internal/core"
	"raisesteady-guard/agent/internal/guard"
	"raisesteady-guard/agent/internal/policy"
)

// Env is what detection reads from Windows.
type Env interface {
	Reg() policy.Registry
	ProgramDirs() []string       // C:\Program Files, C:\Program Files (x86)
	Profiles() map[string]string // SID -> profile folder (C:\Users\name)
	Exists(path string) bool
	ProductName(path string) string
}

// Found is one installed browser.
type Found struct {
	ID         string `json:"id"` // supported id or unsupported id
	Name       string `json:"name"`
	Path       string `json:"path"`
	Supported  bool   `json:"supported"`
	ProfileSID string `json:"profileSid,omitempty"` // installed in this user's profile
}

var machineRel = []string{
	`Google\Chrome\Application\chrome.exe`,
	`Google\Chrome Beta\Application\chrome.exe`,
	`Google\Chrome Dev\Application\chrome.exe`,
	`Microsoft\Edge\Application\msedge.exe`,
	`Microsoft\Edge Beta\Application\msedge.exe`,
	`Microsoft\Edge Dev\Application\msedge.exe`,
	`BraveSoftware\Brave-Browser\Application\brave.exe`,
	`Vivaldi\Application\vivaldi.exe`,
	`Mozilla Firefox\firefox.exe`,
	`Firefox Developer Edition\firefox.exe`,
	`Firefox Nightly\firefox.exe`,
	`Opera\opera.exe`,
	`Waterfox\waterfox.exe`,
	`LibreWolf\librewolf.exe`,
	`Pale Moon\palemoon.exe`,
	`Maxthon\Maxthon.exe`,
	`Floorp\floorp.exe`,
	`Zen Browser\zen.exe`,
	`Chromium\Application\chrome.exe`,
}

var profileRel = []string{
	`AppData\Local\Google\Chrome\Application\chrome.exe`,
	`AppData\Local\Google\Chrome SxS\Application\chrome.exe`,
	`AppData\Local\BraveSoftware\Brave-Browser\Application\brave.exe`,
	`AppData\Local\Vivaldi\Application\vivaldi.exe`,
	`AppData\Local\Mozilla Firefox\firefox.exe`,
	`AppData\Local\Microsoft\Edge SxS\Application\msedge.exe`,
	`AppData\Local\Programs\Opera\opera.exe`,
	`AppData\Local\Programs\Opera GX\opera.exe`,
	`AppData\Local\Programs\Opera Air\opera.exe`,
	`AppData\Local\Yandex\YandexBrowser\Application\browser.exe`,
	`AppData\Local\UCBrowser\Application\UCBrowser.exe`,
	`AppData\Local\Chromium\Application\chrome.exe`,
	`AppData\Local\CentBrowser\Application\chrome.exe`,
	`AppData\Local\Thorium\Application\thorium.exe`,
	`AppData\Local\CocCoc\Browser\Application\browser.exe`,
	`AppData\Local\Naver\Naver Whale\Application\whale.exe`,
	`AppData\Local\Programs\Min\Min.exe`,
	`Desktop\Tor Browser\Browser\firefox.exe`,
	`Downloads\Tor Browser\Browser\firefox.exe`,
	`AppData\Local\Mullvad\MullvadBrowser\Release\mullvadbrowser.exe`,
}

// ExeFromCommand takes the program path out of a shell command line.
func ExeFromCommand(cmd string) string {
	cmd = strings.TrimSpace(cmd)
	if strings.HasPrefix(cmd, `"`) {
		if i := strings.Index(cmd[1:], `"`); i >= 0 {
			return cmd[1 : 1+i]
		}
		return strings.Trim(cmd, `"`)
	}
	if i := strings.Index(strings.ToLower(cmd), ".exe"); i >= 0 {
		return cmd[:i+4]
	}
	return cmd
}

func knownExeNames(list core.UnsupportedList) []string {
	set := map[string]bool{"chrome.exe": true, "msedge.exe": true, "brave.exe": true, "vivaldi.exe": true, "firefox.exe": true, "browser.exe": true}
	for _, b := range list.Browsers {
		for _, e := range b.Exe {
			set[strings.ToLower(e)] = true
		}
	}
	out := make([]string, 0, len(set))
	for e := range set {
		out = append(out, e)
	}
	sort.Strings(out)
	return out
}

// Detect lists the installed browsers.
func Detect(env Env, list core.UnsupportedList) []Found {
	type cand struct{ path, sid string }
	var cands []cand
	for _, d := range env.ProgramDirs() {
		for _, r := range machineRel {
			cands = append(cands, cand{d + `\` + r, ""})
		}
	}
	profiles := env.Profiles()
	sids := make([]string, 0, len(profiles))
	for s := range profiles {
		sids = append(sids, s)
	}
	sort.Strings(sids)
	for _, sid := range sids {
		for _, r := range profileRel {
			cands = append(cands, cand{profiles[sid] + `\` + r, sid})
		}
	}
	reg := env.Reg()
	readCmds := func(root, base, sid string) {
		subs, err := reg.SubKeys(root, base)
		if err != nil {
			return
		}
		for _, s := range subs {
			if v, err := reg.Get(root, base+`\`+s+`\shell\open\command`, ""); err == nil && v.Kind == policy.String {
				cands = append(cands, cand{ExeFromCommand(v.S), sid})
			}
		}
	}
	readCmds("HKLM", `SOFTWARE\Clients\StartMenuInternet`, "")
	readCmds("HKLM", `SOFTWARE\WOW6432Node\Clients\StartMenuInternet`, "")
	exes := knownExeNames(list)
	appPaths := func(root, sid string) {
		for _, e := range exes {
			if v, err := reg.Get(root, `SOFTWARE\Microsoft\Windows\CurrentVersion\App Paths\`+e, ""); err == nil && v.Kind == policy.String {
				cands = append(cands, cand{ExeFromCommand(v.S), sid})
			}
		}
	}
	appPaths("HKLM", "")
	for _, sid := range sids {
		root := policy.RootForSID(sid)
		if reg.RootAvailable(root) {
			readCmds(root, `Software\Clients\StartMenuInternet`, sid)
			appPaths(root, sid)
		}
	}
	seen := map[string]bool{}
	var out []Found
	for _, c := range cands {
		p := strings.TrimSpace(c.path)
		key := strings.ToLower(p)
		if p == "" || seen[key] || !strings.HasSuffix(key, ".exe") {
			continue
		}
		seen[key] = true
		if !env.Exists(p) {
			continue
		}
		prod := env.ProductName(p)
		m := guard.Classify(list, p, prod)
		switch {
		case m.Unsupported != nil:
			out = append(out, Found{ID: m.Unsupported.ID, Name: m.Unsupported.Name, Path: p, ProfileSID: c.sid})
		case m.SupportedID != "":
			name := m.SupportedID
			for _, b := range policy.Browsers {
				if b.ID == m.SupportedID {
					name = b.Name
				}
			}
			out = append(out, Found{ID: m.SupportedID, Name: name, Path: p, Supported: true, ProfileSID: c.sid})
		}
	}
	return out
}
