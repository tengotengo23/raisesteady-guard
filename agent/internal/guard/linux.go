package guard

import (
	"path"
	"strings"

	"raisesteady-guard/agent/internal/core"
)

// Linux process paths are the program file (/proc/<pid>/exe), or
// "flatpak:<app id>" for a Flatpak app installed for the whole computer and
// "flatpak-user:<app id>" for one installed in a home folder.

// LinuxSupported are the install folders whose browsers read our policy
// files: the deb and rpm packages, the Firefox and Chromium snaps, and the
// Firefox Flatpak installed for the whole computer.
var LinuxSupported = []struct{ Prefix, ID string }{
	{"/opt/google/chrome/", "chrome"},
	{"/opt/google/chrome-beta/", "chrome"},
	{"/opt/google/chrome-unstable/", "chrome"},
	{"/usr/lib/chromium/", "chromium"},
	{"/usr/lib/chromium-browser/", "chromium"},
	{"/usr/lib64/chromium-browser/", "chromium"},
	{"/snap/chromium/", "chromium"},
	{"/opt/microsoft/msedge/", "edge"},
	{"/opt/microsoft/msedge-beta/", "edge"},
	{"/opt/microsoft/msedge-dev/", "edge"},
	{"/opt/brave.com/brave/", "brave"},
	{"/opt/brave.com/brave-beta/", "brave"},
	{"/opt/brave.com/brave-nightly/", "brave"},
	{"/usr/lib/firefox/", "firefox"},
	{"/usr/lib64/firefox/", "firefox"},
	{"/usr/lib/firefox-esr/", "firefox"},
	{"/opt/firefox/", "firefox"}, // Mozilla's own download, unpacked by an administrator
	{"/snap/firefox/", "firefox"},
	{"flatpak:org.mozilla.firefox", "firefox"},
}

// LinuxSupportedID returns the supported browser id for a path, or "".
func LinuxSupportedID(p string) string {
	for _, s := range LinuxSupported {
		if strings.HasSuffix(s.Prefix, "/") {
			if strings.HasPrefix(p, s.Prefix) {
				return s.ID
			}
		} else if p == s.Prefix {
			return s.ID
		}
	}
	return ""
}

// ClassifyLinux decides what a Linux program is. Supported install folders
// come first; then Flatpak ids, snap names, parts of the path and program
// names from unsupported-browsers.json. A supported browser's own program
// name found anywhere else (a copy in a home folder, an AppImage) matches its
// "other copy" entry.
func ClassifyLinux(list core.UnsupportedList, p, _ string) Match {
	if p == "" {
		return Match{}
	}
	if inner, ok := strings.CutPrefix(p, "unverified:"); ok {
		// A supported browser that cannot see our policy files.
		want := map[string]string{"chrome": "chrome-other", "chromium": "chromium", "edge": "edge-other", "brave": "brave-other", "firefox": "firefox-other"}[LinuxSupportedID(inner)]
		for i := range list.Browsers {
			if list.Browsers[i].ID == want {
				return Match{Unsupported: &list.Browsers[i]}
			}
		}
		return Match{}
	}
	if id := LinuxSupportedID(p); id != "" {
		if id == "chromium" {
			id = "chrome" // same policies, same extension
		}
		return Match{SupportedID: id}
	}
	if rest, ok := strings.CutPrefix(p, "flatpak-claim:"); ok {
		// A process whose root says it is a Flatpak app: a browser id closes
		// it; otherwise its program path decides.
		id, exe, _ := strings.Cut(rest, ":")
		for i := range list.Browsers {
			for _, f := range list.Browsers[i].Flatpak {
				if strings.EqualFold(f, id) {
					return Match{Unsupported: &list.Browsers[i]}
				}
			}
		}
		if strings.HasPrefix(exe, "flatpak") || strings.HasPrefix(exe, "unverified:") {
			return Match{}
		}
		return ClassifyLinux(list, exe, "")
	}
	var flatpakID, snapName string
	switch {
	case strings.HasPrefix(p, "flatpak:"):
		flatpakID = strings.TrimPrefix(p, "flatpak:")
	case strings.HasPrefix(p, "flatpak-user:"):
		flatpakID = strings.TrimPrefix(p, "flatpak-user:")
	case strings.HasPrefix(p, "/snap/"):
		snapName, _, _ = strings.Cut(strings.TrimPrefix(p, "/snap/"), "/")
	}
	lp := strings.ToLower(p)
	exe := strings.ToLower(path.Base(p))
	for i := range list.Browsers {
		u := &list.Browsers[i]
		for _, f := range u.Flatpak {
			if flatpakID != "" && strings.EqualFold(f, flatpakID) {
				return Match{Unsupported: u}
			}
		}
		for _, s := range u.Snap {
			if snapName != "" && strings.EqualFold(s, snapName) {
				return Match{Unsupported: u}
			}
		}
		for _, pc := range u.LinuxPath {
			if pc != "" && strings.Contains(lp, strings.ToLower(pc)) {
				return Match{Unsupported: u}
			}
		}
	}
	if flatpakID != "" {
		return Match{} // a Flatpak app that is not a browser
	}
	for i := range list.Browsers {
		u := &list.Browsers[i]
		for _, e := range u.LinuxExe {
			if strings.EqualFold(exe, e) {
				return Match{Unsupported: u}
			}
		}
	}
	return Match{}
}
