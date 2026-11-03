package policy

import (
	"encoding/json"
	"strings"
	"testing"

	"raisesteady-guard/agent/internal/core"
)

const (
	sidA = "S-1-5-21-1111111111-2222222222-3333333333-1001"
	sidB = "S-1-5-21-1111111111-2222222222-3333333333-1002"
)

func testData(t *testing.T) *core.Data {
	t.Helper()
	d, err := core.LoadData()
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func testCfg(d *core.Data, band string) core.Config {
	b, _ := d.Bands.Get(band)
	return core.Config{
		Version: 1, InstallID: "abc123", Port: 47615, Mode: core.ModeUsers,
		ExtIDs: core.ExtIDs{ChromiumDevID: "jihggkhiiiinnflomjjnpomgeijgfnlg", FirefoxID: "guard@raisesteady.com"},
		Accounts: []core.Account{
			{SID: sidA, Name: "kid", Token: strings.Repeat("a", 64), Settings: core.SettingsForBand(b)},
		},
	}
}

func targetsFor(cfg core.Config) []Target {
	var ts []Target
	for _, a := range cfg.Accounts {
		root := RootForSID(a.SID)
		if cfg.Mode == core.ModeAll {
			root = "HKLM"
		}
		ts = append(ts, Target{Root: root, Account: a, Lang: "en"})
	}
	return ts
}

func find(es []Entry, key, name string) (Entry, bool) {
	for _, e := range es {
		if strings.EqualFold(e.Key, key) && e.Name == name {
			return e, true
		}
	}
	return Entry{}, false
}

func hasList(es []Entry, key, item string) bool {
	for _, e := range es {
		if e.List && strings.EqualFold(e.Key, key) && e.Val.S == item {
			return true
		}
	}
	return false
}

func TestBuildChromiumPoliciesPerBand(t *testing.T) {
	d := testData(t)
	for _, tc := range []struct {
		band string
		yt   uint32
	}{{"3-5", 2}, {"6-8", 2}, {"9-12", 2}, {"13-15", 1}, {"16-17", 1}} {
		cfg := testCfg(d, tc.band)
		es := Build(Inputs{Cfg: cfg, Data: d}, targetsFor(cfg))
		devtools := uint32(2)
		if tc.band == "13-15" || tc.band == "16-17" {
			devtools = 0 // teenagers: DevTools except on force-installed extensions
		}
		for _, b := range []string{`Software\Policies\Google\Chrome`, `Software\Policies\BraveSoftware\Brave`, `Software\Policies\Microsoft\Edge`} {
			want := map[string]Value{
				"IncognitoModeAvailability": DW(1), "BrowserGuestModeEnabled": DW(0), "BrowserAddPersonEnabled": DW(0),
				"ForceGoogleSafeSearch": DW(1), "ForceYouTubeRestrict": DW(tc.yt), "DnsOverHttpsMode": Str("secure"),
				"DnsOverHttpsTemplates": Str(core.DoHTemplate), "SafeBrowsingProtectionLevel": DW(2), "DownloadRestrictions": DW(1),
				"AllowDeletingBrowserHistory": DW(0), "DeveloperToolsAvailability": DW(devtools), "DefaultPopupsSetting": DW(2),
				"DefaultNotificationsSetting": DW(2), "ExtensionDeveloperModeSettings": DW(1),
				"ProxyMode": Str("direct"), "ProxySettings": Str(`{"ProxyMode":"direct"}`),
			}
			if strings.Contains(b, "Edge") {
				for _, n := range []string{"IncognitoModeAvailability", "BrowserAddPersonEnabled", "SafeBrowsingProtectionLevel"} {
					delete(want, n)
				}
			}
			for n, v := range want {
				e, ok := find(es, b, n)
				if !ok || !e.Val.Equal(v) || e.Root != RootForSID(sidA) {
					t.Errorf("%s %s %s = %+v (found %v), want %+v", tc.band, b, n, e.Val, ok, v)
				}
			}
			id := "jihggkhiiiinnflomjjnpomgeijgfnlg"
			if !hasList(es, b+`\ExtensionInstallForcelist`, id+";"+core.ChromeUpdateURL) {
				t.Errorf("%s %s: no force-install entry", tc.band, b)
			}
			if !hasList(es, b+`\ExtensionInstallBlocklist`, "*") || !hasList(es, b+`\ExtensionInstallAllowlist`, id) {
				t.Errorf("%s %s: block or allow list missing", tc.band, b)
			}
			mk := b + `\3rdparty\extensions\` + id + `\policy`
			for n, v := range map[string]Value{"agentPort": DW(47615), "agentToken": Str(strings.Repeat("a", 64)), "ageBand": Str(tc.band), "focusMode": DW(1), "adblock": DW(1), "installId": Str("abc123")} {
				if e, ok := find(es, mk, n); !ok || !e.Val.Equal(v) {
					t.Errorf("%s %s managed %s = %+v, want %+v", tc.band, b, n, e.Val, v)
				}
			}
		}
		if _, ok := find(es, `Software\Policies\Google\Chrome`, "ForceBingSafeSearch"); ok {
			t.Error("Chrome has no ForceBingSafeSearch policy")
		}
		for _, n := range []string{"TorDisabled", "BraveVPNDisabled"} {
			if e, ok := find(es, `Software\Policies\BraveSoftware\Brave`, n); !ok || !e.Val.Equal(DW(1)) {
				t.Errorf("Brave: %s missing", n)
			}
		}
		if e, _ := find(es, `Software\Policies\Mozilla\Firefox\Proxy`, "Mode"); !e.Val.Equal(Str("none")) {
			t.Error("Firefox Proxy Mode must be none")
		}
		if e, _ := find(es, `Software\Policies\Mozilla\Firefox\Proxy`, "Locked"); !e.Val.Equal(DW(1)) {
			t.Error("Firefox Proxy must be locked")
		}
		for _, e := range es {
			if strings.Contains(e.Key, "Vivaldi") {
				t.Fatal("Vivaldi is not a supported browser")
			}
		}
		_, ffDev := find(es, `Software\Policies\Mozilla\Firefox`, "DisableDeveloperTools")
		if ffDev != (devtools == 2) {
			t.Errorf("%s: Firefox DisableDeveloperTools set=%v", tc.band, ffDev)
		}
	}
}

func TestBuildEdgeNames(t *testing.T) {
	d := testData(t)
	cfg := testCfg(d, "6-8")
	es := Build(Inputs{Cfg: cfg, Data: d}, targetsFor(cfg))
	k := `Software\Policies\Microsoft\Edge`
	for n, v := range map[string]Value{
		"InPrivateModeAvailability": DW(1), "BrowserAddProfileEnabled": DW(0), "ForceBingSafeSearch": DW(2),
		"SmartScreenEnabled": DW(1), "PreventSmartScreenPromptOverride": DW(1), "HubsSidebarEnabled": DW(0),
	} {
		if e, ok := find(es, k, n); !ok || !e.Val.Equal(v) {
			t.Errorf("Edge %s = %+v, want %+v", n, e.Val, v)
		}
	}
	for _, n := range []string{"IncognitoModeAvailability", "BrowserAddPersonEnabled", "SafeBrowsingProtectionLevel", "ManagedBookmarks"} {
		if _, ok := find(es, k, n); ok {
			t.Errorf("Edge must not get the Chrome name %s", n)
		}
	}
	if len(d.Bookmarks.Folders) > 0 {
		if _, ok := find(es, k, "ManagedFavorites"); !ok {
			t.Error("Edge ManagedFavorites missing")
		}
	}
	// Teenagers keep the Edge sidebar.
	cfg = testCfg(d, "13-15")
	es = Build(Inputs{Cfg: cfg, Data: d}, targetsFor(cfg))
	if _, ok := find(es, k, "HubsSidebarEnabled"); ok {
		t.Error("13-15 should keep the sidebar")
	}
}

func TestBuildStoreIDs(t *testing.T) {
	d := testData(t)
	cfg := testCfg(d, "9-12")
	cfg.ExtIDs.ChromeStoreID = strings.Repeat("b", 32)
	cfg.ExtIDs.EdgeStoreID = strings.Repeat("c", 32)
	es := Build(Inputs{Cfg: cfg, Data: d}, targetsFor(cfg))
	if !hasList(es, `Software\Policies\Google\Chrome\ExtensionInstallForcelist`, strings.Repeat("b", 32)+";"+core.ChromeUpdateURL) {
		t.Error("Chrome should use the Chrome Web Store id")
	}
	if !hasList(es, `Software\Policies\Microsoft\Edge\ExtensionInstallForcelist`, strings.Repeat("c", 32)+";"+core.EdgeUpdateURL) {
		t.Error("Edge should use the Edge Add-ons id")
	}
	if _, ok := find(es, `Software\Policies\Microsoft\Edge\3rdparty\extensions\`+strings.Repeat("c", 32)+`\policy`, "agentToken"); !ok {
		t.Error("Edge managed config must sit under the Edge store id")
	}
}

func TestBuildFirefox(t *testing.T) {
	d := testData(t)
	cfg := testCfg(d, "9-12")
	es := Build(Inputs{Cfg: cfg, Data: d}, targetsFor(cfg))
	k := `Software\Policies\Mozilla\Firefox`
	e, ok := find(es, k, "ExtensionSettings")
	if !ok || e.Val.Kind != MultiSZ {
		t.Fatalf("ExtensionSettings missing or not REG_MULTI_SZ: %+v", e.Val)
	}
	if bm, ok := find(es, k, "ManagedBookmarks"); ok && bm.Val.Kind != MultiSZ {
		t.Error("Firefox ManagedBookmarks must be REG_MULTI_SZ")
	}
	if p, _ := find(es, k, "PrivateBrowsingModeAvailability"); !p.Val.Equal(DW(1)) {
		t.Error("PrivateBrowsingModeAvailability missing")
	}
	var es2 map[string]map[string]string
	if err := json.Unmarshal([]byte(e.Val.S), &es2); err != nil {
		t.Fatal(err)
	}
	if es2["*"]["installation_mode"] != "blocked" || es2["guard@raisesteady.com"]["installation_mode"] != "force_installed" || es2["guard@raisesteady.com"]["install_url"] != core.DefaultFirefoxXPI {
		t.Errorf("ExtensionSettings = %v", es2)
	}
	for _, c := range []struct{ key, name string }{
		{k, "DisablePrivateBrowsing"}, {k, "DisableDeveloperTools"}, {k, "BlockAboutConfig"}, {k, "DisableSafeMode"},
		{k + `\DNSOverHTTPS`, "Enabled"}, {k + `\DNSOverHTTPS`, "Locked"},
		{k + `\3rdparty\Extensions\guard@raisesteady.com`, "agentToken"},
	} {
		if _, ok := find(es, c.key, c.name); !ok {
			t.Errorf("Firefox %s\\%s missing", c.key, c.name)
		}
	}
	if e, _ := find(es, k+`\DNSOverHTTPS`, "ProviderURL"); e.Val.S != core.DoHTemplate {
		t.Errorf("DoH provider = %q", e.Val.S)
	}
}

// The family DNS switch: off removes every DoH entry from Chromium and Firefox,
// on (the zero value, so old config files stay on) writes them.
func TestFamilyDNSSwitch(t *testing.T) {
	d := testData(t)
	cfg := testCfg(d, "9-12")
	on := Build(Inputs{Cfg: cfg, Data: d}, targetsFor(cfg))
	for i := range cfg.Accounts {
		cfg.Accounts[i].FamilyDNSOff = true
	}
	off := Build(Inputs{Cfg: cfg, Data: d}, targetsFor(cfg))
	ff := `Software\Policies\Mozilla\Firefox\DNSOverHTTPS`
	for _, b := range Browsers {
		if b.ID == "firefox" {
			continue
		}
		if _, ok := find(on, b.Key, "DnsOverHttpsMode"); !ok {
			t.Errorf("%s: DoH missing while on", b.ID)
		}
		if _, ok := find(off, b.Key, "DnsOverHttpsMode"); ok {
			t.Errorf("%s: DoH written while off", b.ID)
		}
		if _, ok := find(off, b.Key, "DnsOverHttpsTemplates"); ok {
			t.Errorf("%s: DoH template written while off", b.ID)
		}
		if _, ok := find(off, b.Key, "ForceGoogleSafeSearch"); !ok {
			t.Errorf("%s: other policies must stay when DNS is off", b.ID)
		}
	}
	if _, ok := find(on, ff, "ProviderURL"); !ok {
		t.Error("Firefox DoH missing while on")
	}
	for _, n := range []string{"Enabled", "ProviderURL", "Locked", "Fallback"} {
		if _, ok := find(off, ff, n); ok {
			t.Errorf("Firefox DoH %s written while off", n)
		}
	}
	if len(off) != len(on)-2*(len(Browsers)-1)-4 {
		t.Errorf("off should drop exactly the DoH entries: on %d off %d", len(on), len(off))
	}
}

func TestBookmarksPerBand(t *testing.T) {
	bm := core.Bookmarks{Folders: []core.BookmarkFolder{{ID: "learn", Title: map[string]string{"en": "Learn", "ka": "სწავლა"}, Items: []core.Bookmark{
		{Title: "Khan Academy", URL: "https://www.khanacademy.org/", Bands: []string{"6-8", "9-12"}},
		{Title: "Kids only", URL: "https://example.org/kids", Bands: []string{"3-5"}},
		{Title: "Not https", URL: "http://example.org/", Bands: []string{"6-8"}},
	}}}}
	tree := bookmarkTree(bm, "6-8", "ka")
	if !strings.Contains(tree, "Khan Academy") || strings.Contains(tree, "Kids only") || strings.Contains(tree, "Not https") {
		t.Errorf("tree for 6-8 = %s", tree)
	}
	if !strings.Contains(tree, `"toplevel_name":"ისწავლე და ითამაშე"`) || !strings.Contains(tree, "სწავლა") {
		t.Errorf("Georgian names missing: %s", tree)
	}
	if bookmarkTree(bm, "16-17", "en") != "" {
		t.Error("a band without bookmarks should get no policy")
	}
	if bookmarkTree(core.Bookmarks{}, "6-8", "en") != "" {
		t.Error("empty bookmarks should give no policy")
	}
}

func TestModeAllWritesHKLM(t *testing.T) {
	d := testData(t)
	cfg := testCfg(d, "9-12")
	cfg.Mode = core.ModeAll
	cfg.Accounts[0].SID = core.AllSID
	es := Build(Inputs{Cfg: cfg, Data: d}, targetsFor(cfg))
	for _, e := range es {
		if e.Root != "HKLM" {
			t.Fatalf("mode all wrote to %s", e.Root)
		}
	}
	reg := NewFake()
	var st State
	r := Apply(reg, &st, es)
	if r.Written != len(es) || len(r.Errors) > 0 {
		t.Fatalf("apply: %+v (entries %d)", r, len(es))
	}
}

func TestPerUserRootsAndPending(t *testing.T) {
	d := testData(t)
	cfg := testCfg(d, "9-12")
	b, _ := d.Bands.Get("9-12")
	cfg.Accounts = append(cfg.Accounts, core.Account{SID: sidB, Name: "kid2", Token: strings.Repeat("b", 64), Settings: core.SettingsForBand(b)})
	es := Build(Inputs{Cfg: cfg, Data: d}, targetsFor(cfg))
	reg := NewFake(RootForSID(sidA)) // only kid's hive is loaded
	var st State
	r := Apply(reg, &st, es)
	if r.Pending != len(es)/2 || r.Written != len(es)/2 {
		t.Fatalf("want half written, half pending: %+v of %d", r, len(es))
	}
	for k := range reg.Dump() {
		if !strings.HasPrefix(k, `HKU\`+sidA+`\`) {
			t.Fatalf("wrote outside the loaded hive: %s", k)
		}
	}
	if v, _ := reg.Get(RootForSID(sidA), `Software\Policies\Google\Chrome\3rdparty\extensions\jihggkhiiiinnflomjjnpomgeijgfnlg\policy`, "agentToken"); v.S != strings.Repeat("a", 64) {
		t.Error("each account must get its own token")
	}
	// kid2 signs in: the rest is written, nothing of kid1 is rewritten.
	reg.Roots[RootForSID(sidB)] = true
	before := reg.Writes
	r = Apply(reg, &st, es)
	if r.Written != len(es)/2 || r.Pending != 0 || reg.Writes-before != len(es)/2 {
		t.Fatalf("second apply: %+v, writes %d", r, reg.Writes-before)
	}
}

func TestRewriteOnlyOnChange(t *testing.T) {
	d := testData(t)
	cfg := testCfg(d, "9-12")
	reg := NewFake(RootForSID(sidA))
	var st State
	es := Build(Inputs{Cfg: cfg, Data: d}, targetsFor(cfg))
	Apply(reg, &st, es)
	w := reg.Writes
	r := Apply(reg, &st, es)
	if reg.Writes != w || r.Written != 0 || r.Unchanged != len(es) {
		t.Fatalf("unchanged config must not write: %+v, writes %d", r, reg.Writes-w)
	}
	// Someone (or something) changes one value: only that one is put back.
	root := RootForSID(sidA)
	_ = reg.Set(root, `Software\Policies\Google\Chrome`, "IncognitoModeAvailability", DW(0))
	w = reg.Writes
	r = Apply(reg, &st, es)
	if r.Written != 1 || reg.Writes-w != 1 {
		t.Fatalf("one changed value: %+v", r)
	}
	// Band 9-12 to 13-15: only the values that differ are written.
	b, _ := d.Bands.Get("13-15")
	cfg.Accounts[0].Settings = core.SettingsForBand(b)
	es2 := Build(Inputs{Cfg: cfg, Data: d}, targetsFor(cfg))
	old := map[string]Value{}
	for _, e := range es {
		old[e.id()] = e.Val
	}
	diff := 0
	for _, e := range es2 {
		if v, ok := old[e.id()]; !ok || !v.Equal(e.Val) {
			diff++
		}
	}
	r = Apply(reg, &st, es2)
	if r.Written != diff || diff == 0 {
		t.Fatalf("band change wrote %d values, %d differ", r.Written, diff)
	}
	if v, _ := reg.Get(root, `Software\Policies\Google\Chrome`, "ForceYouTubeRestrict"); v.D != 1 {
		t.Errorf("YouTube restrict after band change = %d, want 1", v.D)
	}
	if v, err := reg.Get(root, `Software\Policies\Microsoft\Edge`, "HubsSidebarEnabled"); err == nil {
		t.Errorf("HubsSidebarEnabled should be removed for 13-15, is %+v", v)
	}
}

func TestUninstallRemovesOnlyOurs(t *testing.T) {
	d := testData(t)
	cfg := testCfg(d, "6-8")
	root := RootForSID(sidA)
	reg := NewFake(root)
	chrome := `Software\Policies\Google\Chrome`
	// Policies that were there before us.
	_ = reg.Set(root, chrome, "HomepageLocation", Str("https://school.example/"))
	_ = reg.Set(root, chrome, "ForceGoogleSafeSearch", DW(0))
	_ = reg.Set(root, chrome, "BrowserGuestModeEnabled", DW(0)) // already what we want
	_ = reg.Set(root, chrome+`\ExtensionInstallForcelist`, "1", Str("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa;"+core.ChromeUpdateURL))
	_ = reg.Set(root, chrome+`\ExtensionInstallBlocklist`, "1", Str("*")) // the same item as ours
	_ = reg.Set("HKLM", `Software\Policies\Microsoft\Edge`, "HomepageLocation", Str("https://x.example/"))
	before := reg.Dump()
	keysBefore := reg.KeyCount()

	var st State
	es := Build(Inputs{Cfg: cfg, Data: d}, targetsFor(cfg))
	r := Apply(reg, &st, es)
	if len(r.Errors) > 0 {
		t.Fatal(r.Errors)
	}
	if v, _ := reg.Get(root, chrome+`\ExtensionInstallForcelist`, "2"); !strings.HasPrefix(v.S, "jihggkhiiiinnflomjjnpomgeijgfnlg;") {
		t.Errorf("our force-install entry should be item 2, got %+v", v)
	}
	if v, _ := reg.Get(root, chrome, "ForceGoogleSafeSearch"); v.D != 1 {
		t.Error("safe search not forced")
	}
	// The other extension goes away meanwhile: the list must stay 1..n.
	_ = reg.DeleteValue(root, chrome+`\ExtensionInstallForcelist`, "1")

	r = RemoveAll(reg, &st)
	if len(r.Errors) > 0 || len(st.Records) != 0 {
		t.Fatalf("remove: %+v, %d records left", r, len(st.Records))
	}
	after := reg.Dump()
	delete(before, `HKU\`+sidA+`\`+chrome+`\ExtensionInstallForcelist\1`)
	for k, v := range before {
		if got, ok := after[k]; !ok || !got.Equal(v) {
			t.Errorf("%s: was %+v, after uninstall %+v (present %v)", k, v, got, ok)
		}
	}
	for k := range after {
		if _, ok := before[k]; !ok {
			t.Errorf("left behind: %s", k)
		}
	}
	if reg.KeyCount() > keysBefore {
		t.Errorf("keys left behind: %d before, %d after", keysBefore, reg.KeyCount())
	}
}

func TestListCompaction(t *testing.T) {
	reg := NewFake()
	k := `Software\Policies\Google\Chrome\ExtensionInstallForcelist`
	_ = reg.Set("HKLM", k, "1", Str("a"))
	_ = reg.Set("HKLM", k, "2", Str("b"))
	var st State
	Apply(reg, &st, []Entry{{Root: "HKLM", Key: k, Val: Str("ours"), List: true}})
	_ = reg.DeleteValue("HKLM", k, "1")
	_ = reg.Set("HKLM", k, "4", Str("later"))
	RemoveAll(reg, &st)
	names, _ := reg.ValueNames("HKLM", k)
	if strings.Join(names, ",") != "1,2" {
		t.Fatalf("names after removal = %v", names)
	}
	v1, _ := reg.Get("HKLM", k, "1")
	v2, _ := reg.Get("HKLM", k, "2")
	if v1.S != "b" || v2.S != "later" {
		t.Errorf("order not kept: %q %q", v1.S, v2.S)
	}
}

func TestAliasKeepsLogicalRoot(t *testing.T) {
	reg := NewFake(`HKU\KG_` + sidA)
	a := Alias{Registry: reg, Map: map[string]string{RootForSID(sidA): `HKU\KG_` + sidA}}
	var st State
	r := Apply(a, &st, []Entry{{Root: RootForSID(sidA), Key: `Software\Policies\Vivaldi`, Name: "X", Val: DW(1)}})
	if r.Written != 1 || st.Records[0].Root != RootForSID(sidA) {
		t.Fatalf("%+v %+v", r, st.Records)
	}
	if _, err := reg.Get(`HKU\KG_`+sidA, `Software\Policies\Vivaldi`, "X"); err != nil {
		t.Fatal("not written to the mounted hive")
	}
}
