package guard

import (
	"testing"

	"raisesteady-guard/agent/internal/core"
)

func TestClassifyLinux(t *testing.T) {
	d, err := core.LoadData()
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct{ path, supported, unsupported string }{
		{"/opt/google/chrome/chrome", "chrome", ""},
		{"/opt/google/chrome/chrome_crashpad_handler", "chrome", ""},
		{"/snap/chromium/3000/usr/lib/chromium-browser/chrome", "chrome", ""},
		{"/usr/lib64/chromium-browser/chromium-browser", "chrome", ""},
		{"/opt/microsoft/msedge/msedge", "edge", ""},
		{"/opt/brave.com/brave/brave", "brave", ""},
		{"/usr/lib/firefox-esr/firefox-esr", "firefox", ""},
		{"/snap/firefox/5000/usr/lib/firefox/firefox", "firefox", ""},
		{"/opt/firefox/firefox", "firefox", ""},
		{"flatpak:org.mozilla.firefox", "firefox", ""},
		{"flatpak-user:org.mozilla.firefox", "", "firefox-other"},
		{"flatpak:com.brave.Browser", "", "brave-other"},
		{"flatpak:org.gnome.Epiphany", "", "epiphany"},
		{"flatpak:com.spotify.Client", "", ""},
		{"/snap/brave/400/opt/brave.com/brave/brave", "", "brave-other"},
		{"/snap/vlc/3000/usr/bin/vlc", "", ""},
		{"/tmp/.mount_Thorium1/usr/bin/thorium", "", "thorium"},
		{"/home/kid/tor-browser/Browser/firefox.real", "", "tor"},
		{"/home/kid/firefox/firefox", "", "firefox-other"},
		{"/opt/vivaldi/vivaldi-bin", "", "vivaldi"},
		{"/usr/lib/x86_64-linux-gnu/opera/opera", "", "opera"},
		{"/usr/bin/bash", "", ""},
		{"/usr/share/code/code", "", ""},
		{"unverified:/opt/google/chrome/chrome", "", "chrome-other"},
		{"flatpak-claim:com.spotify.Client:/app/extra/spotify", "", ""},
		{"flatpak-claim:com.spotify.Client:/home/kid/x/chromium", "", "chromium"},
		{"flatpak-claim:com.opera.Opera:/usr/bin/bwrap", "", "opera"},
		{"flatpak-claim:x:flatpak:org.mozilla.firefox", "", ""},
		{"unverified:/usr/lib/chromium/chromium", "", "chromium"},
		{"unverified:/snap/firefox/1/usr/lib/firefox/firefox", "", "firefox-other"},
	}
	for _, c := range cases {
		m := ClassifyLinux(d.Unsupported, c.path, "")
		got := ""
		if m.Unsupported != nil {
			got = m.Unsupported.ID
		}
		if m.SupportedID != c.supported || got != c.unsupported {
			t.Errorf("%s: supported %q unsupported %q, want %q %q", c.path, m.SupportedID, got, c.supported, c.unsupported)
		}
	}
}

// The Linux-only entries never match on Windows.
func TestLinuxEntriesDoNotChangeWindows(t *testing.T) {
	d, err := core.LoadData()
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{`C:\Program Files\Google\Chrome\Application\chrome.exe`, `C:\Program Files\Mozilla Firefox\firefox.exe`} {
		m := Classify(d.Unsupported, p, "")
		if m.Unsupported != nil {
			t.Errorf("%s classified as %s", p, m.Unsupported.ID)
		}
	}
}
