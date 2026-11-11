package core

import (
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"
)

// The ids the stores publish RaiseSteady Guard under (P2-194). The policies
// take them from shared/ext-ids.json; this test fails when that file names
// anything else.
const (
	publishedChromeID  = "kjfgpfhdfdilbkibcegclnbnlfkokcke"
	publishedFirefoxID = "guard@raisesteady.com"
	publishedAMOSlug   = "raisesteady-guard"
)

func TestExtIDsAreThePublishedOnes(t *testing.T) {
	d, err := LoadData()
	if err != nil {
		t.Fatal(err)
	}
	if d.ExtIDs.Placeholder {
		t.Fatal("ext-ids.json did not load: placeholders in use")
	}
	if d.ExtIDs.ChromeStoreID != publishedChromeID {
		t.Errorf("chromeStoreId = %q, the Chrome Web Store item is %q", d.ExtIDs.ChromeStoreID, publishedChromeID)
	}
	if d.ExtIDs.FirefoxID != publishedFirefoxID {
		t.Errorf("firefox id = %q, the add-on is %q", d.ExtIDs.FirefoxID, publishedFirefoxID)
	}
	if d.ExtIDs.FirefoxSlug != publishedAMOSlug {
		t.Errorf("amoSlug = %q, want %q", d.ExtIDs.FirefoxSlug, publishedAMOSlug)
	}
	if got, want := d.ExtIDs.FirefoxStoreURL(), "https://addons.mozilla.org/firefox/downloads/latest/raisesteady-guard/latest.xpi"; got != want {
		t.Errorf("FirefoxStoreURL = %q, want %q", got, want)
	}
	if ChromeUpdateURL != "https://clients2.google.com/service/update2/crx" {
		t.Errorf("ChromeUpdateURL = %q", ChromeUpdateURL)
	}
}

func TestFirefoxStoreURLNeedsAGoodSlug(t *testing.T) {
	for _, bad := range []string{"", "a/b", "../x", "a b", "a?b", strings.Repeat("a", 200)} {
		if ValidSlug(bad) {
			t.Errorf("ValidSlug(%q) = true", bad)
		}
		e := ExtIDs{FirefoxSlug: bad}
		if u := e.FirefoxStoreURL(); strings.Contains(u, bad) && bad != "" {
			t.Errorf("FirefoxStoreURL with slug %q = %q", bad, u)
		}
	}
	if !ValidSlug(publishedAMOSlug) {
		t.Errorf("ValidSlug(%q) = false", publishedAMOSlug)
	}
}

// versionLess compares dotted version numbers (0.4.2 < 0.4.10).
func versionLess(a, b string) bool {
	as, bs := strings.Split(a, "."), strings.Split(b, ".")
	for i := 0; i < len(as) || i < len(bs); i++ {
		var x, y int
		if i < len(as) {
			x, _ = strconv.Atoi(as[i])
		}
		if i < len(bs) {
			y, _ = strconv.Atoi(bs[i])
		}
		if x != y {
			return x < y
		}
	}
	return false
}

func TestVersionLess(t *testing.T) {
	for _, c := range []struct {
		a, b string
		want bool
	}{{"0.4.1", "0.4.2", true}, {"0.4.2", "0.4.2", false}, {"0.4.10", "0.4.2", false}, {"0.3.2.1", "0.4", true}, {"0.4", "0.3.2.1", false}} {
		if got := versionLess(c.a, c.b); got != c.want {
			t.Errorf("versionLess(%s, %s) = %v", c.a, c.b, got)
		}
	}
}

func fetch(t *testing.T, u string) []byte {
	t.Helper()
	c := &http.Client{Timeout: 30 * time.Second}
	req, _ := http.NewRequest(http.MethodGet, u, nil)
	req.Header.Set("User-Agent", "raisesteady-guard-store-check")
	resp, err := c.Do(req)
	if err != nil {
		t.Fatalf("GET %s: %v", u, err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET %s: %s", u, resp.Status)
	}
	return b
}

// TestStoresPublishTheseIDs asks the two stores themselves. It needs the
// internet, so it runs only with KG_ONLINE=1 (the Linux workflow and the
// release check set it):
//
//	KG_ONLINE=1 go test ./internal/core -run TestStoresPublishTheseIDs -v
//
// With KG_EXPECT_EXT_VERSION=0.4.2 it also fails while a store still serves
// an older version than the package expects (run it before a release).
func TestStoresPublishTheseIDs(t *testing.T) {
	if os.Getenv("KG_ONLINE") != "1" {
		t.Skip("needs the internet: set KG_ONLINE=1")
	}
	d, err := LoadData()
	if err != nil {
		t.Fatal(err)
	}
	expect := os.Getenv("KG_EXPECT_EXT_VERSION")

	t.Run("Chrome Web Store", func(t *testing.T) {
		q := url.Values{}
		q.Set("response", "updatecheck")
		q.Set("acceptformat", "crx2,crx3")
		q.Set("prodversion", "131.0.0.0")
		q.Set("x", "id="+d.ExtIDs.ChromeStoreID+"&uc")
		var out struct {
			Apps []struct {
				ID     string `xml:"appid,attr"`
				Status string `xml:"status,attr"`
				Check  struct {
					Status   string `xml:"status,attr"`
					Version  string `xml:"version,attr"`
					Codebase string `xml:"codebase,attr"`
				} `xml:"updatecheck"`
			} `xml:"app"`
		}
		body := fetch(t, ChromeUpdateURL+"?"+q.Encode())
		if err := xml.Unmarshal(body, &out); err != nil {
			t.Fatalf("the store's answer is not XML: %v", err)
		}
		if len(out.Apps) != 1 || out.Apps[0].ID != d.ExtIDs.ChromeStoreID {
			t.Fatalf("the store answered for %v, asked for %s", out.Apps, d.ExtIDs.ChromeStoreID)
		}
		a := out.Apps[0]
		if a.Status != "ok" || a.Check.Status != "ok" || a.Check.Codebase == "" || a.Check.Version == "" {
			t.Fatalf("the Chrome Web Store has no installable item %s: %+v", a.ID, a)
		}
		t.Logf("Chrome Web Store: %s is at version %s", a.ID, a.Check.Version)
		if expect != "" && versionLess(a.Check.Version, expect) {
			t.Errorf("the Chrome Web Store serves %s, the package expects %s", a.Check.Version, expect)
		}
	})

	t.Run("addons.mozilla.org", func(t *testing.T) {
		var out struct {
			GUID    string `json:"guid"`
			Slug    string `json:"slug"`
			Status  string `json:"status"`
			Current struct {
				Version string `json:"version"`
			} `json:"current_version"`
		}
		body := fetch(t, fmt.Sprintf("https://addons.mozilla.org/api/v5/addons/addon/%s/", d.ExtIDs.FirefoxSlug))
		if err := json.Unmarshal(body, &out); err != nil {
			t.Fatalf("the store's answer is not JSON: %v", err)
		}
		if out.GUID != d.ExtIDs.FirefoxID {
			t.Fatalf("addons.mozilla.org/%s is %q, the policy installs %q", d.ExtIDs.FirefoxSlug, out.GUID, d.ExtIDs.FirefoxID)
		}
		if out.Status != "public" || out.Current.Version == "" {
			t.Fatalf("the add-on is not public: status %q, version %q", out.Status, out.Current.Version)
		}
		t.Logf("addons.mozilla.org: %s is at version %s", out.GUID, out.Current.Version)
		if expect != "" && versionLess(out.Current.Version, expect) {
			t.Errorf("addons.mozilla.org serves %s, the package expects %s", out.Current.Version, expect)
		}
	})
}
