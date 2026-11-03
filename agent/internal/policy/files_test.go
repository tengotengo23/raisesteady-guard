//go:build !windows

package policy

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"raisesteady-guard/agent/internal/core"
)

func allCfg(d *core.Data, band string, dnsOff bool) core.Config {
	cfg := testCfg(d, band)
	cfg.Mode = core.ModeAll
	cfg.Accounts[0].SID = core.AllSID
	cfg.Accounts[0].Settings.FamilyDNSOff = dnsOff
	return cfg
}

func newFiles(t *testing.T) (*FileRegistry, string) {
	t.Helper()
	root := t.TempDir()
	r, err := OpenFileRegistry(root, filepath.Join(root, "var/lib/raisesteady-guard/policy-image.json"))
	if err != nil {
		t.Fatal(err)
	}
	return r, root
}

func readJSON(t *testing.T, p string) map[string]any {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("%s: %v", p, err)
	}
	return m
}

func applyFlush(t *testing.T, r *FileRegistry, st *State, desired []Entry) Result {
	t.Helper()
	res := Apply(r, st, desired)
	if len(res.Errors) > 0 {
		t.Fatalf("apply: %v", res.Errors)
	}
	if err := r.Flush(); err != nil {
		t.Fatalf("flush: %v", err)
	}
	return res
}

// Every policy of every band, with the family filter on and off, has a JSON type.
func TestEveryPolicyHasAJSONType(t *testing.T) {
	d := testData(t)
	for _, b := range d.Bands.List {
		for _, off := range []bool{false, true} {
			cfg := allCfg(d, b.ID, off)
			r, _ := newFiles(t)
			var st State
			Apply(r, &st, Build(Inputs{Cfg: cfg, Data: d}, targetsFor(cfg)))
			snap := r.snapshot()
			for _, ft := range LinuxTargets() {
				if _, err := Document(snap, ft.Key, ft.Firefox); err != nil {
					t.Errorf("band %s dnsOff=%v %s: %v", b.ID, off, ft.Key, err)
				}
			}
		}
	}
}

func TestLinuxFilesAndTypes(t *testing.T) {
	defer setUmask(0o077)()
	d := testData(t)
	cfg := allCfg(d, "9-12", false)
	r, root := newFiles(t)
	var st State
	applyFlush(t, r, &st, Build(Inputs{Cfg: cfg, Data: d}, targetsFor(cfg)))

	for _, p := range []string{"etc/opt/chrome", "etc/chromium", "etc/chromium-browser", "etc/opt/edge", "etc/brave"} {
		m := readJSON(t, filepath.Join(root, p, "policies/managed", OurFile))
		if m["ForceGoogleSafeSearch"] != true {
			t.Errorf("%s ForceGoogleSafeSearch = %v, want true", p, m["ForceGoogleSafeSearch"])
		}
		if m["DownloadRestrictions"] != float64(1) {
			t.Errorf("%s DownloadRestrictions = %v, want 1", p, m["DownloadRestrictions"])
		}
		if m["DnsOverHttpsMode"] != "secure" {
			t.Errorf("%s DnsOverHttpsMode = %v", p, m["DnsOverHttpsMode"])
		}
		fl, ok := m["ExtensionInstallForcelist"].([]any)
		if !ok || len(fl) != 1 || !strings.HasPrefix(fl[0].(string), "jihggkhiiiinnflomjjnpomgeijgfnlg;") {
			t.Errorf("%s ExtensionInstallForcelist = %v", p, m["ExtensionInstallForcelist"])
		}
		if bl, _ := m["ExtensionInstallBlocklist"].([]any); len(bl) != 1 || bl[0] != "*" {
			t.Errorf("%s ExtensionInstallBlocklist = %v", p, m["ExtensionInstallBlocklist"])
		}
		if ps, _ := m["ProxySettings"].(map[string]any); ps["ProxyMode"] != "direct" {
			t.Errorf("%s ProxySettings = %v", p, m["ProxySettings"])
		}
		ext := m["3rdparty"].(map[string]any)["extensions"].(map[string]any)
		var mc map[string]any
		for _, v := range ext {
			mc = v.(map[string]any)
		}
		if mc["agentPort"] != float64(47615) || mc["ageBand"] != "9-12" || mc["focusMode"] != true || len(mc["agentToken"].(string)) != 64 {
			t.Errorf("%s managed config = %v", p, mc)
		}
		if _, nested := mc["policy"]; nested {
			t.Errorf("%s: the Windows 'policy' level must not appear on Linux", p)
		}
	}
	edge := readJSON(t, filepath.Join(root, "etc/opt/edge/policies/managed", OurFile))
	if edge["InPrivateModeAvailability"] != float64(1) || edge["ForceBingSafeSearch"] != float64(2) {
		t.Errorf("edge = %v", edge)
	}
	if bm, ok := edge["ManagedFavorites"].([]any); !ok || len(bm) == 0 {
		t.Errorf("edge ManagedFavorites = %v", edge["ManagedFavorites"])
	}
	ff := readJSON(t, filepath.Join(root, "etc/firefox/policies/policies.json"))["policies"].(map[string]any)
	if ff["DisablePrivateBrowsing"] != true || ff["PrivateBrowsingModeAvailability"] != float64(1) {
		t.Errorf("firefox private browsing = %v %v", ff["DisablePrivateBrowsing"], ff["PrivateBrowsingModeAvailability"])
	}
	if doh := ff["DNSOverHTTPS"].(map[string]any); doh["Locked"] != true || doh["Enabled"] != true || doh["Fallback"] != false {
		t.Errorf("firefox DNSOverHTTPS = %v", doh)
	}
	es := ff["ExtensionSettings"].(map[string]any)
	if es["guard@raisesteady.com"].(map[string]any)["installation_mode"] != "force_installed" {
		t.Errorf("firefox ExtensionSettings = %v", es)
	}
	if mc := ff["3rdparty"].(map[string]any)["Extensions"].(map[string]any)["guard@raisesteady.com"].(map[string]any); mc["agentPort"] != float64(47615) || mc["adblock"] != true {
		t.Errorf("firefox managed config = %v", mc)
	}
	if _, ok := ff["ManagedBookmarks"].([]any); !ok {
		t.Errorf("firefox ManagedBookmarks = %v", ff["ManagedBookmarks"])
	}
	// Readable by every user (the browsers run as the child), whatever the umask.
	fi, err := os.Stat(filepath.Join(root, "etc/opt/chrome/policies/managed", OurFile))
	if err != nil || fi.Mode().Perm() != 0o644 {
		t.Errorf("chrome file mode = %v, %v", fi.Mode(), err)
	}
	for _, d := range []string{"etc/opt", "etc/opt/chrome", "etc/opt/chrome/policies", "etc/opt/chrome/policies/managed"} {
		if fi, err := os.Stat(filepath.Join(root, d)); err != nil || fi.Mode().Perm() != 0o755 {
			t.Errorf("%s mode = %v, %v", d, fi.Mode(), err)
		}
	}
	// Conditional targets are not written when their browser is missing.
	if _, err := os.Stat(filepath.Join(root, "etc/firefox-esr")); err == nil {
		t.Error("firefox-esr written without Firefox ESR")
	}
}

func TestFamilyDNSOffOnLinux(t *testing.T) {
	d := testData(t)
	cfg := allCfg(d, "13-15", true)
	r, root := newFiles(t)
	var st State
	applyFlush(t, r, &st, Build(Inputs{Cfg: cfg, Data: d}, targetsFor(cfg)))
	m := readJSON(t, filepath.Join(root, "etc/opt/chrome/policies/managed", OurFile))
	if _, ok := m["DnsOverHttpsTemplates"]; ok {
		t.Error("DNS off still wrote the DoH template")
	}
	if m["DeveloperToolsAvailability"] != float64(0) {
		t.Errorf("13-15 DeveloperToolsAvailability = %v", m["DeveloperToolsAvailability"])
	}
	ff := readJSON(t, filepath.Join(root, "etc/firefox/policies/policies.json"))["policies"].(map[string]any)
	if _, ok := ff["DNSOverHTTPS"]; ok {
		t.Error("DNS off still wrote Firefox DoH")
	}
}

func listFiles(t *testing.T, root string) []string {
	t.Helper()
	var out []string
	_ = filepath.Walk(root, func(p string, fi os.FileInfo, err error) error {
		if err == nil && p != root {
			rel, _ := filepath.Rel(root, p)
			out = append(out, rel)
		}
		return nil
	})
	sort.Strings(out)
	return out
}

func TestRemovalLeavesNothingAndKeepsTheirFirefoxPolicies(t *testing.T) {
	d := testData(t)
	cfg := allCfg(d, "6-8", false)
	r, root := newFiles(t)
	ffPath := filepath.Join(root, "etc/firefox/policies/policies.json")
	if err := os.MkdirAll(filepath.Dir(ffPath), 0o755); err != nil {
		t.Fatal(err)
	}
	theirs := `{"policies": {"DisableAppUpdate": true, "DisablePrivateBrowsing": false}}`
	if err := os.WriteFile(ffPath, []byte(theirs), 0o644); err != nil {
		t.Fatal(err)
	}
	before := listFiles(t, root)
	var st State
	applyFlush(t, r, &st, Build(Inputs{Cfg: cfg, Data: d}, targetsFor(cfg)))
	ff := readJSON(t, ffPath)["policies"].(map[string]any)
	if ff["DisableAppUpdate"] != true || ff["DisablePrivateBrowsing"] != true {
		t.Fatalf("merged firefox = %v", ff)
	}
	applyFlush(t, r, &st, nil) // remove everything
	ff = readJSON(t, ffPath)["policies"].(map[string]any)
	if len(ff) != 2 || ff["DisableAppUpdate"] != true || ff["DisablePrivateBrowsing"] != false {
		t.Errorf("after removal firefox = %v, want their two policies back", ff)
	}
	after := listFiles(t, root)
	// Only the image file (in the data folder, deleted by uninstall) may be new.
	var extra []string
	have := map[string]bool{}
	for _, p := range before {
		have[p] = true
	}
	for _, p := range after {
		if !have[p] && !strings.HasPrefix(p, "var") {
			extra = append(extra, p)
		}
	}
	if len(extra) > 0 {
		t.Errorf("left after removal: %v", extra)
	}
}

func TestOwnFirefoxFileIsDeletedOnRemoval(t *testing.T) {
	d := testData(t)
	cfg := allCfg(d, "3-5", false)
	r, root := newFiles(t)
	var st State
	applyFlush(t, r, &st, Build(Inputs{Cfg: cfg, Data: d}, targetsFor(cfg)))
	applyFlush(t, r, &st, nil)
	for _, p := range listFiles(t, root) {
		if strings.HasPrefix(p, "etc") {
			t.Errorf("left after removal: %s", p)
		}
	}
}

func TestImageSurvivesRestartAndFilesAreRepaired(t *testing.T) {
	d := testData(t)
	cfg := allCfg(d, "9-12", false)
	r, root := newFiles(t)
	var st State
	desired := Build(Inputs{Cfg: cfg, Data: d}, targetsFor(cfg))
	applyFlush(t, r, &st, desired)
	chrome := filepath.Join(root, "etc/opt/chrome/policies/managed", OurFile)
	want, _ := os.ReadFile(chrome)

	// A new process opens the saved image: nothing to write.
	r2, err := OpenFileRegistry(root, r.Image)
	if err != nil {
		t.Fatal(err)
	}
	res := applyFlush(t, r2, &st, desired)
	if res.Written != 0 || res.Unchanged == 0 {
		t.Errorf("after restart: written %d, unchanged %d", res.Written, res.Unchanged)
	}
	// Someone edits our file: the next flush puts it right.
	if err := os.WriteFile(chrome, []byte(`{"IncognitoModeAvailability": 0}`), 0o644); err != nil {
		t.Fatal(err)
	}
	applyFlush(t, r2, &st, desired)
	if got, _ := os.ReadFile(chrome); string(got) != string(want) {
		t.Error("edited policy file was not restored")
	}
	if len(r2.Written()) != 6 {
		t.Errorf("written files = %v", r2.Paths())
	}
}

func TestConditionalTargets(t *testing.T) {
	d := testData(t)
	cfg := allCfg(d, "9-12", false)
	r, root := newFiles(t)
	esrDir := filepath.Join(root, "usr/lib/firefox-esr")
	flatpak := filepath.Join(root, "var/lib/flatpak/app/org.mozilla.firefox")
	for _, p := range []string{esrDir, flatpak} {
		if err := os.MkdirAll(p, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	var st State
	desired := Build(Inputs{Cfg: cfg, Data: d}, targetsFor(cfg))
	applyFlush(t, r, &st, desired)
	esr := filepath.Join(root, "etc/firefox-esr/policies/policies.json")
	fp := filepath.Join(root, "var/lib/flatpak/extension/org.mozilla.firefox.systemconfig", flatpakArch(), "stable/policies/policies.json")
	for _, p := range []string{esr, fp} {
		if _, err := os.Stat(p); err != nil {
			t.Errorf("missing %s", p)
		}
	}
	// Firefox ESR is uninstalled: its file goes at the next flush.
	if err := os.RemoveAll(esrDir); err != nil {
		t.Fatal(err)
	}
	applyFlush(t, r, &st, desired)
	if _, err := os.Stat(filepath.Join(root, "etc/firefox-esr")); err == nil {
		t.Error("firefox-esr folder left after ESR was removed")
	}
}
