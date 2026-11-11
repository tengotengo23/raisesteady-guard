//go:build !windows

package policy

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"raisesteady-guard/agent/internal/core"
)

// The ids and addresses the stores publish RaiseSteady Guard under. They are
// written out here on purpose: a policy that names anything else installs
// nothing, and its blocklist then switches off the copy a parent installed by
// hand from the store (found on a real Ubuntu PC, 10 Nov 2026, P2-194).
const (
	publishedChromeID  = "kjfgpfhdfdilbkibcegclnbnlfkokcke"
	publishedFirefoxID = "guard@raisesteady.com"
	chromeStoreUpdate  = "https://clients2.google.com/service/update2/crx"
	amoLatest          = "https://addons.mozilla.org/firefox/downloads/latest/raisesteady-guard/latest.xpi"
	devChromeID        = "jihggkhiiiinnflomjjnpomgeijgfnlg"
)

// chromiumDirs are the policy folders of every Chromium-family browser
// protected on Linux, by browser.
var chromiumDirs = map[string]string{
	"Google Chrome":           "etc/opt/chrome",
	"Chromium (deb, rpm)":     "etc/chromium",
	"Chromium (Ubuntu, snap)": "etc/chromium-browser",
	"Microsoft Edge":          "etc/opt/edge",
	"Brave":                   "etc/brave",
	"Yandex Browser":          "etc/opt/yandex/browser",
}

// storeCfg is a configuration as the program builds it: the ids come from
// the embedded data, never from the test.
func storeCfg(d *core.Data) core.Config {
	cfg := allCfg(d, "9-12", false)
	cfg.ExtIDs = d.ExtIDs
	return cfg
}

func linuxInputs(cfg core.Config, d *core.Data) Inputs {
	return Inputs{Cfg: cfg, Data: d, Browsers: LinuxBrowsers(), FirefoxURL: d.ExtIDs.FirefoxStoreURL()}
}

func TestPolicyNamesThePublishedStoreID(t *testing.T) {
	d := testData(t)
	if d.ExtIDs.ChromeStoreID != publishedChromeID {
		t.Fatalf("shared/ext-ids.json chromeStoreId = %q, the Chrome Web Store item is %q", d.ExtIDs.ChromeStoreID, publishedChromeID)
	}
	cfg := storeCfg(d)
	r, root := newFiles(t)
	var st State
	applyFlush(t, r, &st, Build(linuxInputs(cfg, d), targetsFor(cfg)))

	want := publishedChromeID + ";" + chromeStoreUpdate
	for name, dir := range chromiumDirs {
		p := filepath.Join(root, dir, "policies/managed", OurFile)
		raw, err := os.ReadFile(p)
		if err != nil {
			t.Errorf("%s: no policy file: %v", name, err)
			continue
		}
		if strings.Contains(string(raw), devChromeID) {
			t.Errorf("%s: the policy file names the development id", name)
		}
		m := readJSON(t, p)
		if fl, _ := m["ExtensionInstallForcelist"].([]any); len(fl) != 1 || fl[0] != want {
			t.Errorf("%s ExtensionInstallForcelist = %v, want [%s]", name, m["ExtensionInstallForcelist"], want)
		}
		if al, _ := m["ExtensionInstallAllowlist"].([]any); len(al) != 1 || al[0] != publishedChromeID {
			t.Errorf("%s ExtensionInstallAllowlist = %v, want [%s]", name, m["ExtensionInstallAllowlist"], publishedChromeID)
		}
		if bl, _ := m["ExtensionInstallBlocklist"].([]any); len(bl) != 1 || bl[0] != "*" {
			t.Errorf("%s ExtensionInstallBlocklist = %v", name, m["ExtensionInstallBlocklist"])
		}
		third, _ := m["3rdparty"].(map[string]any)
		exts, _ := third["extensions"].(map[string]any)
		mc, ok := exts[publishedChromeID].(map[string]any)
		if len(exts) != 1 || !ok {
			t.Errorf("%s: managed settings are not under the store id: %v", name, exts)
			continue
		}
		if mc["agentPort"] != float64(47615) || len(mc["agentToken"].(string)) != 64 {
			t.Errorf("%s managed settings = %v", name, mc)
		}
	}
}

// Every path the brief lists is written, and nothing is written for a
// browser the policy engine does not know.
func TestLinuxPolicyPaths(t *testing.T) {
	d := testData(t)
	cfg := storeCfg(d)
	r, root := newFiles(t)
	var st State
	applyFlush(t, r, &st, Build(linuxInputs(cfg, d), targetsFor(cfg)))
	want := map[string]bool{FirefoxMainFile: true}
	for _, dir := range chromiumDirs {
		want[filepath.Join(dir, "policies/managed", OurFile)] = true
	}
	got := map[string]bool{}
	for _, f := range listFiles(t, root) {
		if fi, err := os.Stat(filepath.Join(root, f)); err != nil || fi.IsDir() || strings.HasPrefix(f, "var/lib/raisesteady-guard/") {
			continue
		}
		got[f] = true
		if !want[f] {
			t.Errorf("unexpected policy file %s", f)
		}
	}
	for f := range want {
		if !got[f] {
			t.Errorf("missing policy file %s", f)
		}
	}
}

// The Windows list stays as it was: Yandex Browser is a Linux target only.
func TestYandexIsLinuxOnly(t *testing.T) {
	for _, b := range Browsers {
		if b.ID == "yandex" {
			t.Fatal("Yandex Browser is in the Windows browser list")
		}
	}
	lb := LinuxBrowsers()
	if lb[len(lb)-1].ID != "firefox" {
		t.Errorf("Firefox must stay last, got %v", lb)
	}
	found := false
	for _, b := range lb {
		if b.ID == "yandex" && b.Key == YandexKey {
			found = true
		}
	}
	if !found {
		t.Errorf("no Yandex Browser in %v", lb)
	}
	d := testData(t)
	cfg := storeCfg(d)
	for _, e := range Build(Inputs{Cfg: cfg, Data: d}, targetsFor(cfg)) {
		if strings.HasPrefix(e.Key, YandexKey) {
			t.Fatalf("the default (Windows) build writes %s", e.Key)
		}
	}
}

func firefoxInstallURL(t *testing.T, p string) string {
	t.Helper()
	ff := readJSON(t, p)["policies"].(map[string]any)
	es := ff["ExtensionSettings"].(map[string]any)
	if len(es) != 2 {
		t.Errorf("%s ExtensionSettings = %v", p, es)
	}
	if all, _ := es["*"].(map[string]any); all["installation_mode"] != "blocked" {
		t.Errorf("%s: other add-ons are not blocked: %v", p, es["*"])
	}
	our, ok := es[publishedFirefoxID].(map[string]any)
	if !ok || our["installation_mode"] != "force_installed" {
		t.Fatalf("%s: no force-installed %s: %v", p, publishedFirefoxID, es)
	}
	if _, ok := ff["3rdparty"].(map[string]any)["Extensions"].(map[string]any)[publishedFirefoxID]; !ok {
		t.Errorf("%s: managed settings are not under %s", p, publishedFirefoxID)
	}
	u, _ := our["install_url"].(string)
	return u
}

// A snap Firefox cannot read a file of the system (bug 2): by default every
// Firefox gets the add-on from addons.mozilla.org.
func TestFirefoxInstallsFromTheStoreByDefault(t *testing.T) {
	d := testData(t)
	if got := d.ExtIDs.FirefoxStoreURL(); got != amoLatest {
		t.Fatalf("FirefoxStoreURL = %q, want %q", got, amoLatest)
	}
	cfg := storeCfg(d)
	r, root := newFiles(t)
	r.StoreURL = d.ExtIDs.FirefoxStoreURL()
	for _, p := range []string{"usr/lib/firefox-esr", "var/lib/flatpak/app/org.mozilla.firefox", "snap/firefox"} {
		if err := os.MkdirAll(filepath.Join(root, p), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	var st State
	applyFlush(t, r, &st, Build(linuxInputs(cfg, d), targetsFor(cfg)))
	for _, p := range []string{FirefoxMainFile, "etc/firefox-esr/policies/policies.json", flatpakFirefoxFile()} {
		if u := firefoxInstallURL(t, filepath.Join(root, p)); u != amoLatest {
			t.Errorf("%s install_url = %q, want %q", p, u, amoLatest)
		}
	}
}

// The file bundled with the package is for a PC without internet, chosen on
// purpose (--firefox-xpi file:...). Even then a policy file that a snap or
// Flatpak Firefox reads names the store, because those cannot open the file.
func TestBundledFileNeverReachesASandboxedFirefox(t *testing.T) {
	const file = "file:///usr/lib/raisesteady-guard/raisesteady-guard.xpi"
	d := testData(t)
	cfg := storeCfg(d)
	cfg.FirefoxInstallURL = file
	cfg.FirefoxFromFile = true

	// No snap: the deb or rpm Firefox reads the file; the Flatpak one does not.
	r, root := newFiles(t)
	r.StoreURL = d.ExtIDs.FirefoxStoreURL()
	for _, p := range []string{"usr/lib/firefox-esr", "var/lib/flatpak/app/org.mozilla.firefox"} {
		if err := os.MkdirAll(filepath.Join(root, p), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	var st State
	desired := Build(linuxInputs(cfg, d), targetsFor(cfg))
	applyFlush(t, r, &st, desired)
	if u := firefoxInstallURL(t, filepath.Join(root, FirefoxMainFile)); u != file {
		t.Errorf("deb Firefox install_url = %q, want the bundled file", u)
	}
	if u := firefoxInstallURL(t, filepath.Join(root, "etc/firefox-esr/policies/policies.json")); u != file {
		t.Errorf("Firefox ESR install_url = %q, want the bundled file", u)
	}
	if u := firefoxInstallURL(t, filepath.Join(root, flatpakFirefoxFile())); u != amoLatest {
		t.Errorf("Flatpak Firefox install_url = %q, want %q", u, amoLatest)
	}

	// The snap Firefox arrives later (each of the places it can show up in):
	// the shared file switches to the store at the next flush.
	for _, snap := range []string{"snap/firefox", "var/lib/snapd/snap/firefox", "snap/bin/firefox"} {
		r, root := newFiles(t)
		r.StoreURL = d.ExtIDs.FirefoxStoreURL()
		var st State
		applyFlush(t, r, &st, desired)
		if u := firefoxInstallURL(t, filepath.Join(root, FirefoxMainFile)); u != file {
			t.Fatalf("before the snap: install_url = %q", u)
		}
		if err := os.MkdirAll(filepath.Join(root, snap), 0o755); err != nil {
			t.Fatal(err)
		}
		applyFlush(t, r, &st, desired)
		if u := firefoxInstallURL(t, filepath.Join(root, FirefoxMainFile)); u != amoLatest {
			t.Errorf("with %s: install_url = %q, want %q", snap, u, amoLatest)
		}
	}
}

// Yandex Browser's protected mode switches extensions off on a page. The
// policy keeps it off and locked, in Yandex Browser's file only.
func TestYandexProtectedModeStaysOff(t *testing.T) {
	d := testData(t)
	cfg := storeCfg(d)
	r, root := newFiles(t)
	var st State
	applyFlush(t, r, &st, Build(linuxInputs(cfg, d), targetsFor(cfg)))
	for name, dir := range chromiumDirs {
		m := readJSON(t, filepath.Join(root, dir, "policies/managed", OurFile))
		v, has := m["YandexProtectedMode"]
		if name == "Yandex Browser" {
			if v != false {
				t.Errorf("Yandex Browser: YandexProtectedMode = %v, want false", v)
			}
			if m["IncognitoModeAvailability"] != float64(1) {
				t.Errorf("Yandex Browser: IncognitoModeAvailability = %v, want 1 (a forced extension does not run in Incognito)", m["IncognitoModeAvailability"])
			}
		} else if has {
			t.Errorf("%s: YandexProtectedMode must not be written", name)
		}
	}
}
