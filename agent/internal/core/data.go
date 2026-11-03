package core

import (
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"strings"
)

// The files in data/ are copied from shared/ by tools/build-agent.sh.
// Never edit them here.
//
//go:embed data/*.json
var dataFS embed.FS

// Band is one entry of shared/age-bands.json.
type Band struct {
	ID          string  `json:"id"`
	MinAge      int     `json:"minAge"`
	MaxAge      int     `json:"maxAge"`
	BrowseMin   int     `json:"browseMin"`
	VideoMin    int     `json:"videoMin"`
	Bedtime     Bedtime `json:"bedtime"`
	YouTube     string  `json:"youtube"`
	YouTubeMode string  `json:"youtubeMode"`
	Social      string  `json:"social"`
	BreakEvery  int     `json:"breakEveryMin"`
}

// Bands is the parsed age-bands.json.
type Bands struct {
	List []Band `json:"bands"`
}

// Get returns the band with this id.
func (b *Bands) Get(id string) (Band, bool) {
	for _, x := range b.List {
		if x.ID == id {
			return x, true
		}
	}
	return Band{}, false
}

// Bookmark is one item of shared/bookmarks.json.
type Bookmark struct {
	Title string            `json:"title"`
	URL   string            `json:"url"`
	Bands []string          `json:"bands"`
	Desc  map[string]string `json:"desc"`
	Cost  string            `json:"cost"`
}

// BookmarkFolder is one folder of shared/bookmarks.json.
type BookmarkFolder struct {
	ID    string            `json:"id"`
	Title map[string]string `json:"title"`
	Items []Bookmark        `json:"items"`
}

// Bookmarks is the parsed bookmarks.json.
type Bookmarks struct {
	Folders []BookmarkFolder `json:"folders"`
}

// Unsupported is one browser of shared/unsupported-browsers.json.
type Unsupported struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	Exe          []string `json:"exe"`
	Product      []string `json:"product"`
	PathContains []string `json:"pathContains"`
}

// UnsupportedList is the parsed unsupported-browsers.json.
type UnsupportedList struct {
	Browsers []Unsupported `json:"browsers"`
	// SupportedProducts are never closed, whatever else matches.
	SupportedProducts []string `json:"supportedProducts"`
}

type extIDsFile struct {
	Chromium struct {
		DevID         string  `json:"devId"`
		ChromeStoreID *string `json:"chromeStoreId"`
		EdgeStoreID   *string `json:"edgeStoreId"`
	} `json:"chromium"`
	Firefox struct {
		ID string `json:"id"`
	} `json:"firefox"`
}

// Data is everything embedded at build time.
type Data struct {
	Bands       Bands
	Bookmarks   Bookmarks
	Unsupported UnsupportedList
	ExtIDs      ExtIDs
	// Missing lists embedded files that were absent at build time.
	Missing []string
}

// LoadData parses the embedded files. Bookmarks, ext ids and the unsupported list
// may be missing (the build says so); the age bands may not.
func LoadData() (*Data, error) { return loadData(dataFS) }

func loadData(fsys fs.FS) (*Data, error) {
	d := &Data{}
	read := func(name string, v any, required bool) error {
		b, err := fs.ReadFile(fsys, "data/"+name)
		if err != nil {
			if required {
				return fmt.Errorf("embedded %s missing: run tools/build-agent.sh", name)
			}
			d.Missing = append(d.Missing, name)
			return nil
		}
		if len(strings.TrimSpace(string(b))) == 0 {
			d.Missing = append(d.Missing, name)
			return nil
		}
		if err := json.Unmarshal(b, v); err != nil {
			return fmt.Errorf("embedded %s: %w", name, err)
		}
		return nil
	}
	if err := read("age-bands.json", &d.Bands, true); err != nil {
		return nil, err
	}
	if len(d.Bands.List) == 0 {
		return nil, fmt.Errorf("age-bands.json has no bands")
	}
	if err := read("bookmarks.json", &d.Bookmarks, false); err != nil {
		return nil, err
	}
	if err := read("unsupported-browsers.json", &d.Unsupported, false); err != nil {
		return nil, err
	}
	var ids extIDsFile
	if err := read("ext-ids.json", &ids, false); err != nil {
		return nil, err
	}
	d.ExtIDs = ExtIDs{ChromiumDevID: ids.Chromium.DevID, FirefoxID: ids.Firefox.ID}
	if ids.Chromium.ChromeStoreID != nil {
		d.ExtIDs.ChromeStoreID = *ids.Chromium.ChromeStoreID
	}
	if ids.Chromium.EdgeStoreID != nil {
		d.ExtIDs.EdgeStoreID = *ids.Chromium.EdgeStoreID
	}
	if !ValidChromiumID(d.ExtIDs.ChromiumDevID) {
		d.ExtIDs.ChromiumDevID = PlaceholderChromiumID
		d.ExtIDs.Placeholder = true
	}
	if d.ExtIDs.ChromeStoreID != "" && !ValidChromiumID(d.ExtIDs.ChromeStoreID) {
		d.ExtIDs.ChromeStoreID = ""
	}
	if d.ExtIDs.EdgeStoreID != "" && !ValidChromiumID(d.ExtIDs.EdgeStoreID) {
		d.ExtIDs.EdgeStoreID = ""
	}
	if !ValidGeckoID(d.ExtIDs.FirefoxID) {
		d.ExtIDs.FirefoxID = PlaceholderFirefoxID
		d.ExtIDs.Placeholder = true
	}
	return d, nil
}
