package policy

import (
	"encoding/json"
	"strings"

	"raisesteady-guard/agent/internal/core"
)

// Browser is a browser the agent protects through its policies.
type Browser struct {
	ID   string // chrome, edge, brave, firefox
	Name string
	Key  string // policy key below HKLM or HKU\<SID>
}

// Browsers are the supported browsers (Vivaldi is not: its policy support is
// not documented, so the guard closes it in protected accounts), in the order the parent page lists them.
var Browsers = []Browser{
	{"chrome", "Google Chrome", `Software\Policies\Google\Chrome`},
	{"edge", "Microsoft Edge", `Software\Policies\Microsoft\Edge`},
	{"brave", "Brave", `Software\Policies\BraveSoftware\Brave`},
	{"firefox", "Firefox", `Software\Policies\Mozilla\Firefox`},
}

// Target is one place to write policies: HKLM (mode all) or HKU\<SID>.
type Target struct {
	Root    string
	Account core.Account
	Lang    string // en, ka or ru: bookmark folder names
}

// RootForSID returns the registry root of a user's hive.
func RootForSID(sid string) string { return `HKU\` + sid }

// Inputs are what Build needs besides the targets.
type Inputs struct {
	Cfg  core.Config
	Data *core.Data
}

// ChromiumExtension returns the id and update URL to force-install for a browser.
func ChromiumExtension(ids core.ExtIDs, browser, devUpdateURL string) (id, updateURL string) {
	if browser == "edge" && ids.EdgeStoreID != "" {
		return ids.EdgeStoreID, core.EdgeUpdateURL
	}
	if ids.ChromeStoreID != "" {
		return ids.ChromeStoreID, core.ChromeUpdateURL
	}
	if devUpdateURL == "" {
		devUpdateURL = core.ChromeUpdateURL
	}
	return ids.ChromiumDevID, devUpdateURL
}

var folderTitle = map[string]string{
	"en": "Learn and play",
	"ka": "ისწავლე და ითამაშე",
	"ru": "Учись и играй",
}

// bookmarkTree is the Chromium ManagedBookmarks / Firefox ManagedBookmarks JSON
// for a band, or "" when the band has no bookmarks.
func bookmarkTree(bm core.Bookmarks, band, lang string) string {
	type node struct {
		Toplevel string `json:"toplevel_name,omitempty"`
		Name     string `json:"name,omitempty"`
		URL      string `json:"url,omitempty"`
		Children []node `json:"children,omitempty"`
	}
	pick := func(m map[string]string) string {
		if s := m[lang]; s != "" {
			return s
		}
		return m["en"]
	}
	top := folderTitle[lang]
	if top == "" {
		top = folderTitle["en"]
	}
	out := []node{{Toplevel: top}}
	for _, f := range bm.Folders {
		var kids []node
		for _, it := range f.Items {
			if !strings.HasPrefix(it.URL, "https://") {
				continue
			}
			for _, b := range it.Bands {
				if b == band {
					kids = append(kids, node{Name: it.Title, URL: it.URL})
					break
				}
			}
		}
		if len(kids) > 0 {
			out = append(out, node{Name: pick(f.Title), Children: kids})
		}
	}
	if len(out) == 1 {
		return ""
	}
	b, _ := json.Marshal(out)
	return string(b)
}

func ytRestrict(b core.Band) uint32 {
	if b.YouTube == "moderate" {
		return 1
	}
	return 2
}

// Build returns every desired entry for the targets.
func Build(in Inputs, targets []Target) []Entry {
	var out []Entry
	for _, t := range targets {
		band, ok := in.Data.Bands.Get(t.Account.Band)
		if !ok {
			band = in.Data.Bands.List[0]
		}
		for _, b := range Browsers {
			if b.ID == "firefox" {
				out = append(out, firefox(in, t, b, band)...)
			} else {
				out = append(out, chromium(in, t, b, band)...)
			}
		}
	}
	return out
}

func managed(in Inputs, t Target) []struct {
	name string
	v    Value
} {
	port := in.Cfg.Port
	if port == 0 {
		port = core.DefaultPort
	}
	return []struct {
		name string
		v    Value
	}{
		{"agentPort", DW(uint32(port))},
		{"agentToken", Str(t.Account.Token)},
		{"ageBand", Str(t.Account.Band)},
		{"focusMode", Bool(t.Account.FocusMode)},
		{"adblock", Bool(t.Account.Adblock)},
		{"installId", Str(in.Cfg.InstallID)},
	}
}

func chromium(in Inputs, t Target, b Browser, band core.Band) []Entry {
	k := b.Key
	edge := b.ID == "edge"
	ids := in.Cfg.ExtIDs
	id, upd := ChromiumExtension(ids, b.ID, in.Cfg.DevUpdateURL)
	under13 := band.MinAge < 13
	// Under 13: no DevTools at all. Teenagers keep DevTools to learn web coding,
	// except on the pages of force-installed extensions (value 0).
	devtools := uint32(2)
	if !under13 {
		devtools = 0
	}
	s := func(name string, v Value) Entry { return Entry{Root: t.Root, Key: k, Name: name, Val: v} }
	l := func(list, item string) Entry {
		return Entry{Root: t.Root, Key: k + `\` + list, Val: Str(item), List: true}
	}
	alt := func(chrome, edgeName string) string {
		if edge {
			return edgeName
		}
		return chrome
	}
	e := []Entry{
		l("ExtensionInstallForcelist", id+";"+upd),
		l("ExtensionInstallBlocklist", "*"),
		l("ExtensionInstallAllowlist", id),
		s(alt("IncognitoModeAvailability", "InPrivateModeAvailability"), DW(1)),
		s("BrowserGuestModeEnabled", DW(0)),
		s(alt("BrowserAddPersonEnabled", "BrowserAddProfileEnabled"), DW(0)),
		s("ForceGoogleSafeSearch", DW(1)),
		s("ForceYouTubeRestrict", DW(ytRestrict(band))),
		s("DownloadRestrictions", DW(1)),
		s("AllowDeletingBrowserHistory", DW(0)),
		s("DeveloperToolsAvailability", DW(devtools)),
		s("ExtensionDeveloperModeSettings", DW(1)),
		s("DefaultPopupsSetting", DW(2)),
		s("DefaultNotificationsSetting", DW(2)),
		s(alt("BookmarkBarEnabled", "FavoritesBarEnabled"), DW(1)),
		// No proxy: a system proxy set by the user would skip the family DNS.
		// ProxyMode is deprecated in favour of ProxySettings; both are set.
		s("ProxyMode", Str("direct")),
		s("ProxySettings", Str(`{"ProxyMode":"direct"}`)),
	}
	if !t.Account.FamilyDNSOff {
		e = append(e, s("DnsOverHttpsMode", Str("secure")), s("DnsOverHttpsTemplates", Str(core.DoHTemplate)))
	}
	if edge {
		e = append(e,
			s("ForceBingSafeSearch", DW(2)),
			s("SmartScreenEnabled", DW(1)),
			s("PreventSmartScreenPromptOverride", DW(1)),
			s("PreventSmartScreenPromptOverrideForFiles", DW(1)),
		)
		if under13 {
			e = append(e, s("HubsSidebarEnabled", DW(0)))
		}
	} else {
		e = append(e, s("SafeBrowsingProtectionLevel", DW(2)))
	}
	if b.ID == "brave" {
		// Brave's own policies (brave-core policy_definitions/BraveSoftware).
		e = append(e, s("TorDisabled", DW(1)), s("BraveVPNDisabled", DW(1)))
		if under13 {
			e = append(e, s("BraveAIChatEnabled", DW(0)))
		}
	}
	if tree := bookmarkTree(in.Data.Bookmarks, t.Account.Band, t.Lang); tree != "" {
		e = append(e, s(alt("ManagedBookmarks", "ManagedFavorites"), Str(tree)))
	}
	mk := k + `\3rdparty\extensions\` + id + `\policy`
	for _, m := range managed(in, t) {
		e = append(e, Entry{Root: t.Root, Key: mk, Name: m.name, Val: m.v})
	}
	return e
}

func firefox(in Inputs, t Target, b Browser, band core.Band) []Entry {
	k := b.Key
	id := in.Cfg.ExtIDs.FirefoxID
	url := in.Cfg.FirefoxInstallURL
	if url == "" {
		url = core.DefaultFirefoxXPI
	}
	settings := map[string]map[string]string{
		"*": {"installation_mode": "blocked", "blocked_install_message": core.AppName + ": other add-ons are switched off on this account."},
		id:  {"installation_mode": "force_installed", "install_url": url},
	}
	js, _ := json.Marshal(settings)
	s := func(key, name string, v Value) Entry { return Entry{Root: t.Root, Key: key, Name: name, Val: v} }
	e := []Entry{
		// Object and array policies must be REG_MULTI_SZ: Firefox parses
		// MULTI_SZ as JSON, but a REG_SZ stays a string and fails validation.
		s(k, "ExtensionSettings", Multi(string(js))),
		s(k, "PrivateBrowsingModeAvailability", DW(1)), // Firefox 130+
		s(k, "DisablePrivateBrowsing", DW(1)),          // older Firefox
		s(k, "BlockAboutConfig", DW(1)),
		s(k, "DisableSafeMode", DW(1)),
		s(k, "DisableForgetButton", DW(1)),
		s(k+`\Proxy`, "Mode", Str("none")),
		s(k+`\Proxy`, "Locked", DW(1)),
		s(k+`\PopupBlocking`, "Default", DW(1)),
		s(k+`\PopupBlocking`, "Locked", DW(1)),
		s(k+`\Permissions\Notifications`, "BlockNewRequests", DW(1)),
		s(k+`\Permissions\Notifications`, "Locked", DW(1)),
	}
	if !t.Account.FamilyDNSOff {
		e = append(e,
			s(k+`\DNSOverHTTPS`, "Enabled", DW(1)),
			s(k+`\DNSOverHTTPS`, "ProviderURL", Str(core.DoHTemplate)),
			s(k+`\DNSOverHTTPS`, "Locked", DW(1)),
			s(k+`\DNSOverHTTPS`, "Fallback", DW(0)),
		)
	}
	if band.MinAge < 13 {
		e = append(e, s(k, "DisableDeveloperTools", DW(1)))
	}
	if tree := bookmarkTree(in.Data.Bookmarks, t.Account.Band, t.Lang); tree != "" {
		e = append(e, s(k, "ManagedBookmarks", Multi(tree)))
	}
	mk := k + `\3rdparty\Extensions\` + id
	for _, m := range managed(in, t) {
		e = append(e, s(mk, m.name, m.v))
	}
	return e
}
