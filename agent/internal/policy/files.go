package policy

// Linux browsers read policies from JSON files for the whole computer, not
// from a registry. FileRegistry keeps the registry view the engine works with
// (an image saved in the data folder) and renders it to the browsers' files on
// Flush. Each Chromium-family file is wholly ours; Firefox has one shared
// policies.json, so ours are merged into it and taken out again on removal.

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
)

// OurFile is the name of our policy file in each Chromium-family managed folder.
const OurFile = "raisesteady-guard.json"

// FileTarget is one browser's policy files on Linux.
type FileTarget struct {
	Key     string   // the policy key below HKLM, as in Browsers
	Firefox bool     // policies.json format, shared file
	Paths   []string // always written (relative to the root)
	// IfPresent maps a path to write to a path that must exist first.
	IfPresent map[string]string
}

func flatpakArch() string {
	if runtime.GOARCH == "arm64" {
		return "aarch64"
	}
	return "x86_64"
}

// LinuxTargets are the policy files of the supported browsers (deb, rpm and
// snap packages read the same folders; snap Chromium reads chromium-browser).
func LinuxTargets() []FileTarget {
	m := func(dir string) string { return filepath.Join(dir, "policies", "managed", OurFile) }
	return []FileTarget{
		{Key: `Software\Policies\Google\Chrome`, Paths: []string{
			m("etc/opt/chrome"), m("etc/chromium"), m("etc/chromium-browser"),
		}},
		{Key: `Software\Policies\Microsoft\Edge`, Paths: []string{m("etc/opt/edge")}},
		{Key: `Software\Policies\BraveSoftware\Brave`, Paths: []string{m("etc/brave")}},
		{Key: `Software\Policies\Mozilla\Firefox`, Firefox: true,
			Paths: []string{"etc/firefox/policies/policies.json"},
			IfPresent: map[string]string{
				"etc/firefox-esr/policies/policies.json": "usr/lib/firefox-esr",
				filepath.Join("var/lib/flatpak/extension/org.mozilla.firefox.systemconfig", flatpakArch(), "stable/policies/policies.json"): "var/lib/flatpak/app/org.mozilla.firefox",
			}},
	}
}

// JSON types of the DWORD policies (everything else is a number). A name
// missing from both lists stops Flush, so a new policy is always classified.
var boolNames = map[string]bool{
	// Chromium family
	"BrowserGuestModeEnabled": true, "BrowserAddPersonEnabled": true, "BrowserAddProfileEnabled": true,
	"ForceGoogleSafeSearch": true, "AllowDeletingBrowserHistory": true, "BookmarkBarEnabled": true,
	"FavoritesBarEnabled": true, "SmartScreenEnabled": true, "PreventSmartScreenPromptOverride": true,
	"PreventSmartScreenPromptOverrideForFiles": true, "HubsSidebarEnabled": true, "TorDisabled": true,
	"BraveVPNDisabled": true, "BraveAIChatEnabled": true,
	// Firefox
	"DisablePrivateBrowsing": true, "BlockAboutConfig": true, "DisableSafeMode": true,
	"DisableForgetButton": true, "DisableDeveloperTools": true, "Locked": true, "Enabled": true,
	"Fallback": true, "Default": true, "BlockNewRequests": true,
	// extension managed storage (managed_schema.json)
	"focusMode": true, "adblock": true,
}

var numberNames = map[string]bool{
	"IncognitoModeAvailability": true, "InPrivateModeAvailability": true, "ForceYouTubeRestrict": true,
	"DownloadRestrictions": true, "DeveloperToolsAvailability": true, "ExtensionDeveloperModeSettings": true,
	"DefaultPopupsSetting": true, "DefaultNotificationsSetting": true, "SafeBrowsingProtectionLevel": true,
	"ForceBingSafeSearch": true, "PrivateBrowsingModeAvailability": true, "agentPort": true,
}

// String policies whose text is JSON (Windows reads them as JSON text).
var jsonStringNames = map[string]bool{
	"ManagedBookmarks": true, "ManagedFavorites": true, "ProxySettings": true, "ExtensionSettings": true,
}

// JSONValue converts a registry value to its JSON form.
func JSONValue(name string, v Value) (any, error) {
	switch v.Kind {
	case DWord:
		switch {
		case boolNames[name]:
			return v.D != 0, nil
		case numberNames[name]:
			return v.D, nil
		}
		return nil, fmt.Errorf("policy %s has no JSON type (add it to boolNames or numberNames)", name)
	case MultiSZ:
		var x any
		if err := json.Unmarshal([]byte(v.S), &x); err != nil {
			return nil, fmt.Errorf("policy %s: %w", name, err)
		}
		return x, nil
	case String:
		if jsonStringNames[name] {
			var x any
			if err := json.Unmarshal([]byte(v.S), &x); err != nil {
				return nil, fmt.Errorf("policy %s: %w", name, err)
			}
			return x, nil
		}
		return v.S, nil
	}
	return nil, fmt.Errorf("policy %s: unknown value kind %d", name, v.Kind)
}

func isListKey(vals map[string]Value) bool {
	if len(vals) == 0 {
		return false
	}
	for n := range vals {
		if _, err := strconv.Atoi(n); err != nil {
			return false
		}
	}
	return true
}

// Document renders the values below key to the browser's JSON object.
// Subkeys become objects; a subkey whose value names are all numbers becomes
// a list. Chromium's `3rdparty\extensions\<id>\policy` loses the "policy"
// level, as in the Linux file format.
func Document(snap map[string]map[string]Value, key string, firefox bool) (map[string]any, error) {
	doc := map[string]any{}
	pre := strings.ToLower(`HKLM\` + key)
	paths := make([]string, 0, len(snap))
	for p := range snap {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	var errs []string
	for _, p := range paths {
		lp := strings.ToLower(p)
		if lp != pre && !strings.HasPrefix(lp, pre+`\`) {
			continue
		}
		vals := snap[p]
		if len(vals) == 0 {
			continue
		}
		rel := strings.Trim(p[len(pre):], `\`)
		var parts []string
		if rel != "" {
			parts = strings.Split(rel, `\`)
		}
		if !firefox && len(parts) == 4 && strings.EqualFold(parts[0], "3rdparty") && strings.EqualFold(parts[3], "policy") {
			parts = parts[:3]
		}
		if isListKey(vals) && len(parts) > 0 {
			names := make([]string, 0, len(vals))
			for n := range vals {
				names = append(names, n)
			}
			sort.Slice(names, func(i, j int) bool {
				a, _ := strconv.Atoi(names[i])
				b, _ := strconv.Atoi(names[j])
				return a < b
			})
			list := make([]any, 0, len(names))
			for _, n := range names {
				x, err := JSONValue(parts[len(parts)-1], vals[n])
				if err != nil {
					errs = append(errs, err.Error())
					continue
				}
				list = append(list, x)
			}
			parent, err := objectAt(doc, parts[:len(parts)-1])
			if err != nil {
				errs = append(errs, err.Error())
				continue
			}
			parent[parts[len(parts)-1]] = list
			continue
		}
		obj, err := objectAt(doc, parts)
		if err != nil {
			errs = append(errs, err.Error())
			continue
		}
		for n, v := range vals {
			x, err := JSONValue(n, v)
			if err != nil {
				errs = append(errs, err.Error())
				continue
			}
			obj[n] = x
		}
	}
	if len(errs) > 0 {
		sort.Strings(errs)
		return doc, errors.New(strings.Join(errs, "; "))
	}
	return doc, nil
}

func objectAt(doc map[string]any, parts []string) (map[string]any, error) {
	cur := doc
	for _, p := range parts {
		next, ok := cur[p]
		if !ok {
			m := map[string]any{}
			cur[p] = m
			cur = m
			continue
		}
		m, ok := next.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("policy %s is both a value and a key", p)
		}
		cur = m
	}
	return cur, nil
}

// imageFile is the saved state of a FileRegistry.
type imageFile struct {
	Keys map[string]map[string]Value `json:"keys"`
	// Written is the SHA-256 of what we last wrote at each path.
	Written map[string]string `json:"written"`
	// Ours are the Firefox policy names we put into each shared file, and
	// Theirs the values another tool had set for them before (put back on removal).
	Ours   map[string][]string                   `json:"ours,omitempty"`
	Theirs map[string]map[string]json.RawMessage `json:"theirs,omitempty"`
	// Dirs are folders we created (removed again when empty).
	Dirs []string `json:"dirs,omitempty"`
}

// FileRegistry is a Registry whose HKLM policies are written to files.
type FileRegistry struct {
	*Fake
	Root    string // "/" in use; a temporary folder in tests
	Image   string // the saved image (in the data folder)
	Targets []FileTarget
	img     imageFile
}

// OpenFileRegistry loads the saved image, if any.
func OpenFileRegistry(root, image string) (*FileRegistry, error) {
	r := &FileRegistry{Fake: NewFake(), Root: root, Image: image, Targets: LinuxTargets()}
	b, err := os.ReadFile(image)
	switch {
	case errors.Is(err, fs.ErrNotExist):
	case err != nil:
		return nil, err
	default:
		if err := json.Unmarshal(b, &r.img); err != nil {
			return nil, fmt.Errorf("read %s: %w", image, err)
		}
	}
	for k, vals := range r.img.Keys {
		root, key, _ := strings.Cut(k, `\`)
		if !r.Fake.Roots[root] {
			continue
		}
		if len(vals) == 0 {
			r.Fake.mu.Lock()
			r.Fake.keys[fk(root, key)] = map[string]Value{}
			r.Fake.names[fk(root, key)] = k
			r.Fake.mu.Unlock()
		}
		for n, v := range vals {
			_ = r.Fake.Set(root, key, n, v)
		}
	}
	r.Fake.Writes = 0
	return r, nil
}

func (r *FileRegistry) snapshot() map[string]map[string]Value {
	r.Fake.mu.Lock()
	defer r.Fake.mu.Unlock()
	out := map[string]map[string]Value{}
	for k, vals := range r.Fake.keys {
		c := map[string]Value{}
		for n, v := range vals {
			c[n] = v
		}
		out[r.Fake.names[k]] = c
	}
	return out
}

func (r *FileRegistry) abs(rel string) string { return filepath.Join(r.Root, rel) }

func (r *FileRegistry) exists(rel string) bool {
	_, err := os.Stat(r.abs(rel))
	return err == nil
}

func sum(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

// mkdirs creates the folder of rel and records each folder it created.
func (r *FileRegistry) mkdirs(rel string) error {
	dir := filepath.Dir(rel)
	var missing []string
	for d := dir; d != "." && d != "/" && !r.exists(d); d = filepath.Dir(d) {
		missing = append(missing, d)
	}
	for i := len(missing) - 1; i >= 0; i-- {
		if err := os.Mkdir(r.abs(missing[i]), 0o755); err != nil && !errors.Is(err, fs.ErrExist) {
			return err
		}
		// The service runs with UMask=0077; browsers run as the child and must read these.
		if err := os.Chmod(r.abs(missing[i]), 0o755); err != nil {
			return err
		}
		r.img.Dirs = append(r.img.Dirs, missing[i])
	}
	return nil
}

// writeFile writes atomically (temporary file, then rename), readable by all.
func (r *FileRegistry) writeFile(rel string, b []byte) error {
	if cur, err := os.ReadFile(r.abs(rel)); err == nil && bytes.Equal(cur, b) {
		r.img.Written[rel] = sum(b)
		return nil
	}
	if err := r.mkdirs(rel); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(r.abs(rel)), ".raisesteady-guard-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(b); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Chmod(0o644); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmp.Name(), r.abs(rel)); err != nil {
		return err
	}
	r.img.Written[rel] = sum(b)
	return nil
}

func (r *FileRegistry) removeFile(rel string) error {
	if err := os.Remove(r.abs(rel)); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	delete(r.img.Written, rel)
	return nil
}

// removeEmptyDirs removes the folders we created once nothing is left in them.
func (r *FileRegistry) removeEmptyDirs() {
	sort.Slice(r.img.Dirs, func(i, j int) bool { return len(r.img.Dirs[i]) > len(r.img.Dirs[j]) })
	var keep []string
	for _, d := range r.img.Dirs {
		if _, busy := r.dirInUse(d); busy {
			keep = append(keep, d)
			continue
		}
		if err := os.Remove(r.abs(d)); err != nil && !errors.Is(err, fs.ErrNotExist) {
			keep = append(keep, d)
		}
	}
	r.img.Dirs = keep
}

func (r *FileRegistry) dirInUse(d string) (string, bool) {
	for p := range r.img.Written {
		if strings.HasPrefix(p, d+string(filepath.Separator)) {
			return p, true
		}
	}
	return "", false
}

func marshal(v any) []byte {
	b, _ := json.MarshalIndent(v, "", "  ")
	return append(b, '\n')
}

// mergeFirefox puts ours into the shared policies.json at rel (ours empty:
// takes ours out, puts back what another tool had set, and deletes the file
// when nothing else is in it).
func (r *FileRegistry) mergeFirefox(rel string, ours map[string]any) error {
	top := map[string]any{}
	cur, err := os.ReadFile(r.abs(rel))
	switch {
	case errors.Is(err, fs.ErrNotExist):
	case err != nil:
		return err
	default:
		if json.Unmarshal(cur, &top) != nil {
			if len(ours) == 0 {
				return nil // not ours and unreadable: leave it
			}
			// Firefox ignores a broken file; keep a copy next to it.
			if err := os.WriteFile(r.abs(rel)+".broken", cur, 0o644); err != nil {
				return err
			}
			top = map[string]any{}
		}
	}
	pol, _ := top["policies"].(map[string]any)
	if pol == nil {
		pol = map[string]any{}
	}
	prevOurs := map[string]bool{}
	for _, n := range r.img.Ours[rel] {
		prevOurs[n] = true
	}
	theirs := r.img.Theirs[rel]
	// Take out what we put in before, putting back their values.
	for n := range prevOurs {
		if _, still := ours[n]; still {
			continue
		}
		delete(pol, n)
		if raw, ok := theirs[n]; ok {
			var x any
			if json.Unmarshal(raw, &x) == nil {
				pol[n] = x
			}
			delete(theirs, n)
		}
	}
	for n, v := range ours {
		if old, ok := pol[n]; ok && !prevOurs[n] {
			if theirs == nil {
				theirs = map[string]json.RawMessage{}
			}
			raw, _ := json.Marshal(old)
			theirs[n] = raw
		}
		pol[n] = v
	}
	names := make([]string, 0, len(ours))
	for n := range ours {
		names = append(names, n)
	}
	sort.Strings(names)
	if len(names) > 0 {
		r.img.Ours[rel] = names
	} else {
		delete(r.img.Ours, rel)
	}
	if len(theirs) > 0 {
		r.img.Theirs[rel] = theirs
	} else {
		delete(r.img.Theirs, rel)
	}
	if len(pol) == 0 {
		delete(top, "policies")
	} else {
		top["policies"] = pol
	}
	if len(top) == 0 {
		return r.removeFile(rel)
	}
	if len(ours) == 0 {
		// The file is someone else's again.
		if err := r.writeFile(rel, marshal(top)); err != nil {
			return err
		}
		delete(r.img.Written, rel)
		return nil
	}
	return r.writeFile(rel, marshal(top))
}

// Flush renders the image to the policy files and saves the image. It writes
// only files whose content changed, so a file someone edited is put right.
func (r *FileRegistry) Flush() error {
	if r.img.Written == nil {
		r.img.Written = map[string]string{}
	}
	if r.img.Ours == nil {
		r.img.Ours = map[string][]string{}
	}
	if r.img.Theirs == nil {
		r.img.Theirs = map[string]map[string]json.RawMessage{}
	}
	snap := r.snapshot()
	var errs []string
	want := map[string]bool{}
	for _, t := range r.Targets {
		doc, err := Document(snap, t.Key, t.Firefox)
		if err != nil {
			errs = append(errs, err.Error())
		}
		paths := append([]string(nil), t.Paths...)
		for p, need := range t.IfPresent {
			if r.exists(need) {
				paths = append(paths, p)
			}
		}
		sort.Strings(paths)
		for _, p := range paths {
			want[p] = true
			var err error
			switch {
			case t.Firefox:
				err = r.mergeFirefox(p, doc)
			case len(doc) == 0:
				err = r.removeFile(p)
			default:
				err = r.writeFile(p, marshal(doc))
			}
			if err != nil {
				errs = append(errs, err.Error())
			}
		}
	}
	// A path we wrote that is no longer a target (a browser was removed).
	for p := range r.img.Written {
		if want[p] {
			continue
		}
		var err error
		if strings.HasSuffix(p, "policies.json") {
			err = r.mergeFirefox(p, nil)
		} else {
			err = r.removeFile(p)
		}
		if err != nil {
			errs = append(errs, err.Error())
		}
	}
	r.removeEmptyDirs()
	r.img.Keys = snap
	if err := os.MkdirAll(filepath.Dir(r.Image), 0o700); err != nil {
		errs = append(errs, err.Error())
	} else if err := os.WriteFile(r.Image, marshal(r.img), 0o600); err != nil {
		errs = append(errs, err.Error())
	}
	if len(errs) > 0 {
		return errors.New(strings.Join(errs, "; "))
	}
	return nil
}

// Written returns the policy files we wrote: relative path -> SHA-256.
func (r *FileRegistry) Written() map[string]string {
	out := map[string]string{}
	for p, h := range r.img.Written {
		out[p] = h
	}
	return out
}

// Paths returns the policy files currently written (for status and tests).
func (r *FileRegistry) Paths() []string {
	out := make([]string, 0, len(r.img.Written))
	for p := range r.img.Written {
		out = append(out, p)
	}
	sort.Strings(out)
	return out
}
