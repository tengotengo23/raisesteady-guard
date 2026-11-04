// Package core holds the names, configuration, PIN hashing and embedded data
// shared by every part of the RaiseSteady Guard agent.
package core

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// AppName is the one place the agent keeps the display name.
const AppName = "RaiseSteady Guard"

const (
	ServiceName = "RaiseSteadyGuard"
	DefaultPort = 47615
	// ModeUsers protects chosen Windows accounts (per-user policies under HKU\<SID>).
	ModeUsers = "users"
	// ModeAll protects everyone on this PC (machine policies under HKLM).
	ModeAll = "all"
	// AllSID is the pseudo account used in ModeAll.
	AllSID = "*"
	// DoHTemplate is the family DNS-over-HTTPS resolver (AdGuard DNS Family, free).
	DoHTemplate = "https://family.adguard-dns.com/dns-query"
	// Chrome Web Store and Edge Add-ons update URLs for force-installed extensions.
	ChromeUpdateURL = "https://clients2.google.com/service/update2/crx"
	EdgeUpdateURL   = "https://edge.microsoft.com/extensionwebstorebase/v1/crx"
	// Placeholder ids, used only when shared/ext-ids.json was missing at build time.
	PlaceholderChromiumID = "kgkgkgkgkgkgkgkgkgkgkgkgkgkgkgkg"
	PlaceholderFirefoxID  = "guard@raisesteady.com"
	// DefaultFirefoxXPI is where the installer puts the signed Firefox package.
	DefaultFirefoxXPI = "file:///C:/Program%20Files/RaiseSteadyGuard/raisesteady-guard.xpi"
)

// Bedtime is a daily window in local time, "HH:MM"; it may cross midnight.
type Bedtime struct {
	Start string `json:"start"`
	End   string `json:"end"`
}

// Settings are what the parent controls for one protected account.
type Settings struct {
	Band      string   `json:"band"`
	BrowseMin int      `json:"browseMin"`
	VideoMin  int      `json:"videoMin"`
	Bedtime   Bedtime  `json:"bedtime"`
	Allow     []string `json:"allow"`
	Block     []string `json:"block"`
	FocusMode bool     `json:"focusMode"`
	Adblock   bool     `json:"adblock"`
	// FamilyDNSOff turns off the AdGuard DNS Family resolver for this account
	// (zero value = on, so older config files keep it on).
	FamilyDNSOff bool `json:"familyDnsOff,omitempty"`

	// 0.3.0 (P2-160). Every zero value is the old behaviour, so older config
	// files load with these off (searches: on, as the brief sets).

	// Weekend: Saturday and Sunday minutes, and the bedtime of Friday and
	// Saturday nights (a night belongs to the day it starts).
	Weekend DayRule `json:"weekend"`
	// Homework: a window on school days (Monday to Friday) when only learning
	// sites and the parent's allowed sites open.
	Homework Window `json:"homework"`
	// PauseUntil is the end of "Pause the internet" (RFC 3339), "" when not paused.
	PauseUntil string `json:"pauseUntil,omitempty"`
	// SearchesOff stops recording search words for the report.
	SearchesOff bool `json:"searchesOff,omitempty"`
	// GameMin is the daily game time; nil = the age band's video minutes.
	GameMin *int `json:"gameMin,omitempty"`
	// Apps are the parent's choices for the PC apps (id -> AppAllowed,
	// AppGame or AppBlocked); an app not named here takes its default.
	Apps map[string]string `json:"apps,omitempty"`

	// 0.3.1 (P2-161). TopicsOff turns off the topic filter "Sexuality and
	// gender identity topics" in the extension (zero value = on, so older
	// config files load with it on, as the brief sets for ages 3-17).
	TopicsOff bool `json:"topicsOff,omitempty"`
}

// DayRule is an optional set of daily minutes and a bedtime.
type DayRule struct {
	On        bool    `json:"on"`
	BrowseMin int     `json:"browseMin,omitempty"`
	VideoMin  int     `json:"videoMin,omitempty"`
	Bedtime   Bedtime `json:"bedtime"`
}

// Window is an optional same-day window in local time, "HH:MM" to "HH:MM".
type Window struct {
	On    bool   `json:"on"`
	Start string `json:"start,omitempty"`
	End   string `json:"end,omitempty"`
}

// What a PC app may do in an account.
const (
	AppAllowed = "allowed"
	AppGame    = "game"
	AppBlocked = "blocked"
)

// Account is one protected Windows account (or AllSID in ModeAll).
type Account struct {
	SID   string `json:"sid"`
	Name  string `json:"name"`
	Token string `json:"token"`
	Settings
}

// ExtIDs are the extension ids the policies name.
type ExtIDs struct {
	ChromiumDevID string `json:"chromiumDevId"`
	ChromeStoreID string `json:"chromeStoreId,omitempty"`
	EdgeStoreID   string `json:"edgeStoreId,omitempty"`
	FirefoxID     string `json:"firefoxId"`
	Placeholder   bool   `json:"placeholder,omitempty"`
}

// Config is C:\ProgramData\RaiseSteadyGuard\config.json (ACL: SYSTEM and Administrators).
type Config struct {
	Version         int       `json:"version"`
	InstallID       string    `json:"installId"`
	Port            int       `json:"port"`
	Mode            string    `json:"mode"`
	Pin             PinHash   `json:"pin"`
	Accounts        []Account `json:"accounts"`
	SettingsVersion int       `json:"settingsVersion"`
	ExtIDs          ExtIDs    `json:"extIds"`
	// FirefoxInstallURL is the signed XPI (file:// or https://addons.mozilla.org/...).
	FirefoxInstallURL string `json:"firefoxInstallUrl"`
	// DevUpdateURL is used with the dev id when no store id is known.
	DevUpdateURL string `json:"devUpdateUrl,omitempty"`
	// UTCOffsetMin is the fixed clock of the agent (minutes east of UTC),
	// recorded at install and set on the parent page. The day, bedtime and
	// "now" follow it, never the Windows time zone, which a standard user may
	// change. Summer time is not followed.
	UTCOffsetMin int `json:"utcOffsetMin"`
}

// ValidOffset reports whether m is a UTC offset in minutes (-12:00 to +14:00, 15-minute steps).
func ValidOffset(m int) bool { return m >= -720 && m <= 840 && m%15 == 0 }

// Zone returns the fixed time zone of an offset.
func Zone(offsetMin int) *time.Location {
	return time.FixedZone(FormatOffset(offsetMin), offsetMin*60)
}

// FormatOffset writes an offset as "UTC+04:00".
func FormatOffset(m int) string {
	sign := "+"
	if m < 0 {
		sign, m = "-", -m
	}
	return fmt.Sprintf("UTC%s%02d:%02d", sign, m/60, m%60)
}

// SystemOffsetMin is this computer's UTC offset now (used once, at install).
func SystemOffsetMin(t time.Time) int {
	_, off := t.Zone()
	m := off / 60
	return m - m%15
}

// RandomHex returns n random bytes as hex.
func RandomHex(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic("crypto/rand failed: " + err.Error())
	}
	return hex.EncodeToString(b)
}

// NewToken returns a random 64-hex token.
func NewToken() string { return RandomHex(32) }

var (
	sidRe    = regexp.MustCompile(`^S-1-[0-9]+(-[0-9]+){1,15}$`)
	hhmmRe   = regexp.MustCompile(`^([01][0-9]|2[0-3]):[0-5][0-9]$`)
	tokenRe  = regexp.MustCompile(`^[0-9a-f]{64}$`)
	domainRe = regexp.MustCompile(`^([a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?\.)+[a-z0-9-]{2,63}$`)
	chromeID = regexp.MustCompile(`^[a-p]{32}$`)
	appIDRe  = regexp.MustCompile(`^[a-z0-9-]{1,40}$`)
	geckoID  = regexp.MustCompile(`^([A-Za-z0-9._+-]{1,80}@[A-Za-z0-9.-]{1,80}|\{[0-9a-fA-F-]{36}\})$`)
)

// ErrNoProfile means a Windows account has never signed in (no NTUSER.DAT yet).
var ErrNoProfile = errors.New("the account has no profile yet")

// ValidSID reports whether s looks like a Windows SID string.
func ValidSID(s string) bool { return sidRe.MatchString(s) }

// ValidHHMM reports whether s is "HH:MM".
func ValidHHMM(s string) bool { return hhmmRe.MatchString(s) }

// ValidToken reports whether s is 64 lowercase hex characters.
func ValidToken(s string) bool { return tokenRe.MatchString(s) }

// ValidChromiumID reports whether s is a Chromium extension id.
func ValidChromiumID(s string) bool { return chromeID.MatchString(s) }

// ValidGeckoID reports whether s is a Firefox add-on id.
func ValidGeckoID(s string) bool { return geckoID.MatchString(s) }

// NormalizeDomain lowercases, trims a scheme, path, "www." and a trailing dot,
// and returns "" when the result is not a plain host name.
func NormalizeDomain(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	for _, p := range []string{"https://", "http://"} {
		s = strings.TrimPrefix(s, p)
	}
	if i := strings.IndexAny(s, "/?#:"); i >= 0 {
		s = s[:i]
	}
	s = strings.TrimSuffix(s, ".")
	s = strings.TrimPrefix(s, "www.")
	if len(s) > 253 || !domainRe.MatchString(s) {
		return ""
	}
	return s
}

// ValidPin reports whether pin is 4 to 8 digits.
func ValidPin(pin string) bool {
	if len(pin) < 4 || len(pin) > 8 {
		return false
	}
	for _, r := range pin {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// SettingsForBand returns the default settings of a band.
func SettingsForBand(b Band) Settings {
	return Settings{
		Band: b.ID, BrowseMin: b.BrowseMin, VideoMin: b.VideoMin,
		Bedtime: Bedtime{Start: b.Bedtime.Start, End: b.Bedtime.End},
		Allow:   []string{}, Block: []string{}, FocusMode: true, Adblock: true,
	}
}

// Validate checks a configuration before it is saved or used.
func (c *Config) Validate(bands *Bands) error {
	if c.Mode != ModeUsers && c.Mode != ModeAll {
		return fmt.Errorf("mode must be %q or %q", ModeUsers, ModeAll)
	}
	if c.Port < 1024 || c.Port > 65535 {
		return errors.New("port out of range")
	}
	if len(c.Accounts) == 0 {
		return errors.New("no protected account")
	}
	if !ValidOffset(c.UTCOffsetMin) {
		return errors.New("bad UTC offset")
	}
	if c.Mode == ModeAll && (len(c.Accounts) != 1 || c.Accounts[0].SID != AllSID) {
		return errors.New("mode all needs exactly the account *")
	}
	seen := map[string]bool{}
	for _, a := range c.Accounts {
		if c.Mode == ModeUsers && !ValidSID(a.SID) {
			return fmt.Errorf("bad SID %q", a.SID)
		}
		if seen[a.SID] {
			return fmt.Errorf("account listed twice")
		}
		seen[a.SID] = true
		if !ValidToken(a.Token) {
			return errors.New("bad account token")
		}
		if err := a.Settings.Validate(bands); err != nil {
			return err
		}
	}
	return nil
}

// Validate checks one account's settings.
func (s *Settings) Validate(bands *Bands) error {
	if _, ok := bands.Get(s.Band); !ok {
		return fmt.Errorf("unknown age band %q", s.Band)
	}
	if s.BrowseMin < 0 || s.BrowseMin > 1440 || s.VideoMin < 0 || s.VideoMin > 1440 {
		return errors.New("limit out of range")
	}
	if !ValidHHMM(s.Bedtime.Start) || !ValidHHMM(s.Bedtime.End) {
		return errors.New("bedtime must be HH:MM")
	}
	if len(s.Allow) > 500 || len(s.Block) > 500 {
		return errors.New("too many domains")
	}
	for _, d := range append(append([]string{}, s.Allow...), s.Block...) {
		if NormalizeDomain(d) != d {
			return fmt.Errorf("bad domain %q", d)
		}
	}
	if s.Weekend.On {
		w := s.Weekend
		if w.BrowseMin < 0 || w.BrowseMin > 1440 || w.VideoMin < 0 || w.VideoMin > 1440 {
			return errors.New("weekend limit out of range")
		}
		if !ValidHHMM(w.Bedtime.Start) || !ValidHHMM(w.Bedtime.End) {
			return errors.New("weekend bedtime must be HH:MM")
		}
	}
	if s.Homework.On {
		h := s.Homework
		if !ValidHHMM(h.Start) || !ValidHHMM(h.End) || h.Start >= h.End {
			return errors.New("homework time must be HH:MM to a later HH:MM")
		}
	}
	if s.PauseUntil != "" {
		if _, err := time.Parse(time.RFC3339, s.PauseUntil); err != nil {
			return errors.New("bad pause time")
		}
	}
	if s.GameMin != nil && (*s.GameMin < 0 || *s.GameMin > 1440) {
		return errors.New("game time out of range")
	}
	if len(s.Apps) > 200 {
		return errors.New("too many apps")
	}
	for id, m := range s.Apps {
		if !appIDRe.MatchString(id) || (m != AppAllowed && m != AppGame && m != AppBlocked) {
			return fmt.Errorf("bad app choice %q", id)
		}
	}
	return nil
}

// AccountByToken finds the account for a token in constant time per entry.
func (c *Config) AccountByToken(tok string) (int, bool) {
	if !ValidToken(tok) {
		return -1, false
	}
	found := -1
	for i := range c.Accounts {
		if ConstantTimeEqual(c.Accounts[i].Token, tok) {
			found = i
		}
	}
	return found, found >= 0
}

// Clone returns a deep copy.
func (c Config) Clone() Config {
	b, _ := json.Marshal(c)
	var out Config
	_ = json.Unmarshal(b, &out)
	return out
}

// WriteFileAtomic writes data to a temporary file next to path and renames it.
func WriteFileAtomic(path string, data []byte) error {
	dir := filepath.Dir(path)
	f, err := os.CreateTemp(dir, ".tmp-*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	if _, err := f.Write(data); err != nil {
		f.Close()
		os.Remove(tmp)
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		os.Remove(tmp)
		return err
	}
	if err := f.Close(); err != nil {
		os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return err
	}
	return nil
}

// LoadJSON reads path into v.
func LoadJSON(path string, v any) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, v)
}

// SaveJSON writes v to path atomically.
func SaveJSON(path string, v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return WriteFileAtomic(path, b)
}
