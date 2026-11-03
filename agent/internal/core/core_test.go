package core

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
	"time"
)

func TestPinHash(t *testing.T) {
	h, err := HashPin("4821")
	if err != nil {
		t.Fatal(err)
	}
	if !h.Check("4821") || h.Check("4822") || h.Check("") || h.Check("48210000x") {
		t.Fatal("PIN check wrong")
	}
	if strings.Contains(h.Hash+h.Salt, "4821") {
		t.Fatal("PIN visible in the hash")
	}
	h2, _ := HashPin("4821")
	if h2.Salt == h.Salt || h2.Hash == h.Hash {
		t.Fatal("salt must be random")
	}
	if h.Alg != "argon2id" || h.MemKiB < 19*1024 || h.Time < 2 {
		t.Fatalf("weak parameters: %+v", h)
	}
	for _, bad := range []string{"123", "123456789", "12a4", " 1234"} {
		if _, err := HashPin(bad); err == nil {
			t.Errorf("HashPin(%q) should fail", bad)
		}
	}
}

func TestLockout(t *testing.T) {
	now := time.Date(2026, 11, 3, 10, 0, 0, 0, time.UTC)
	path := filepath.Join(t.TempDir(), "lockout.json")
	l := &Lockout{Path: path, Now: func() time.Time { return now }}
	h, _ := HashPin("1234")
	for i := 1; i <= 4; i++ {
		ok, left, _, err := l.Verify(h, "0000")
		if ok || err != nil || left != MaxTries-i {
			t.Fatalf("try %d: ok=%v left=%d err=%v", i, ok, left, err)
		}
	}
	if ok, _, _, err := l.Verify(h, "0000"); ok || !errors.Is(err, ErrLocked) {
		t.Fatal("fifth wrong PIN must lock")
	}
	// Locked: even the right PIN is refused, also through a second instance (the uninstaller).
	l2 := &Lockout{Path: path, Now: func() time.Time { return now.Add(14 * time.Minute) }}
	if ok, _, retry, err := l2.Verify(h, "1234"); ok || !errors.Is(err, ErrLocked) || retry <= 0 {
		t.Fatal("right PIN must be refused while locked")
	}
	now = now.Add(LockFor + time.Second)
	if ok, _, _, err := l.Verify(h, "1234"); !ok || err != nil {
		t.Fatalf("after 15 minutes the right PIN works: %v %v", ok, err)
	}
	// A success resets the count.
	l.Verify(h, "0000")
	l.Verify(h, "1234")
	if _, left, _, _ := l.Verify(h, "0000"); left != MaxTries-1 {
		t.Fatalf("count not reset: %d", left)
	}
}

func TestValidation(t *testing.T) {
	for in, want := range map[string]string{
		"Example.ORG": "example.org", "https://www.khanacademy.org/math": "khanacademy.org", "a.b.c.example.co.uk": "a.b.c.example.co.uk",
		"localhost": "", "exa mple.com": "", "javascript:alert(1)": "", "": "", "127.0.0.1": "", "x.com.": "x.com",
	} {
		if got := NormalizeDomain(in); got != want {
			t.Errorf("NormalizeDomain(%q) = %q, want %q", in, got, want)
		}
	}
	if !ValidSID("S-1-5-21-1-2-3-1001") || ValidSID("S-1-5-21-1-2-3-1001;rm") || ValidSID("*") {
		t.Error("ValidSID")
	}
	if !ValidHHMM("07:00") || ValidHHMM("24:00") || ValidHHMM("7:00") {
		t.Error("ValidHHMM")
	}
}

func TestConfigValidateAndToken(t *testing.T) {
	d, err := LoadData()
	if err != nil {
		t.Fatal(err)
	}
	b, _ := d.Bands.Get("9-12")
	c := Config{Mode: ModeUsers, Port: DefaultPort, Accounts: []Account{
		{SID: "S-1-5-21-1-2-3-1001", Token: NewToken(), Settings: SettingsForBand(b)},
		{SID: "S-1-5-21-1-2-3-1002", Token: NewToken(), Settings: SettingsForBand(b)},
	}}
	if err := c.Validate(&d.Bands); err != nil {
		t.Fatal(err)
	}
	if i, ok := c.AccountByToken(c.Accounts[1].Token); !ok || i != 1 {
		t.Fatal("token lookup")
	}
	if _, ok := c.AccountByToken(strings.Repeat("0", 64)); ok {
		t.Fatal("unknown token accepted")
	}
	c.Accounts[1].Allow = []string{"Bad Domain"}
	if c.Validate(&d.Bands) == nil {
		t.Fatal("bad domain accepted")
	}
}

func TestLoadDataMissingAndPlaceholders(t *testing.T) {
	fsys := fstest.MapFS{
		"data/age-bands.json": {Data: []byte(`{"bands":[{"id":"6-8","browseMin":90}]}`)},
		"data/bookmarks.json": {Data: []byte(``)},
		"data/ext-ids.json":   {Data: []byte(`{"chromium":{"devId":"not-an-id","chromeStoreId":null},"firefox":{"id":""}}`)},
	}
	d, err := loadData(fsys)
	if err != nil {
		t.Fatal(err)
	}
	if !d.ExtIDs.Placeholder || d.ExtIDs.ChromiumDevID != PlaceholderChromiumID || d.ExtIDs.FirefoxID != PlaceholderFirefoxID {
		t.Fatalf("placeholders: %+v", d.ExtIDs)
	}
	if strings.Join(d.Missing, ",") != "bookmarks.json,unsupported-browsers.json" {
		t.Fatalf("missing = %v", d.Missing)
	}
	if _, err := loadData(fstest.MapFS{}); err == nil {
		t.Fatal("age bands are required")
	}
}

func TestEmbeddedData(t *testing.T) {
	d, err := LoadData()
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Bands.List) != 5 {
		t.Errorf("bands = %d", len(d.Bands.List))
	}
	if len(d.Unsupported.Browsers) < 20 {
		t.Errorf("unsupported browsers = %d", len(d.Unsupported.Browsers))
	}
	t.Logf("ext ids placeholder=%v, bookmark folders=%d, missing=%v", d.ExtIDs.Placeholder, len(d.Bookmarks.Folders), d.Missing)
}

func TestLockoutSourcesAndToday(t *testing.T) {
	now := time.Date(2026, 11, 3, 10, 0, 0, 0, time.UTC)
	l := &Lockout{Path: filepath.Join(t.TempDir(), "l.json"), Now: func() time.Time { return now }}
	h, _ := HashPin("1234")
	for i := 0; i < MaxTries; i++ {
		l.VerifyKey("api:kid", h, "0000")
	}
	if _, _, _, err := l.VerifyKey("api:kid", h, "1234"); !errors.Is(err, ErrLocked) {
		t.Fatal("api source must be locked")
	}
	if ok, _, _, err := l.VerifyKey("parent", h, "1234"); !ok || err != nil {
		t.Fatal("parent source must stay open")
	}
	if n, last := l.Today(); n != MaxTries || !last.Equal(now) {
		t.Fatalf("today: %d %v", n, last)
	}
	now = now.Add(24 * time.Hour)
	if n, _ := l.Today(); n != 0 {
		t.Fatal("the count is per day")
	}
}

func TestOffsets(t *testing.T) {
	if !ValidOffset(240) || !ValidOffset(-570) || !ValidOffset(345) || ValidOffset(241) || ValidOffset(900) {
		t.Fatal("ValidOffset")
	}
	if FormatOffset(330) != "UTC+05:30" || FormatOffset(-210) != "UTC-03:30" || FormatOffset(0) != "UTC+00:00" {
		t.Fatal("FormatOffset")
	}
	if SystemOffsetMin(time.Date(2026, 1, 1, 0, 0, 0, 0, time.FixedZone("x", 4*3600))) != 240 {
		t.Fatal("SystemOffsetMin")
	}
}
