// Package policy builds the browser policies for each protected account and
// writes them to the registry, changing only what differs and removing only
// what it wrote itself.
package policy

import (
	"errors"
	"sort"
	"strings"
	"sync"
)

// Kind is a registry value type.
type Kind int

const (
	String Kind = 1 // REG_SZ
	DWord  Kind = 4 // REG_DWORD
	// MultiSZ is REG_MULTI_SZ; S holds the strings joined by "\n".
	MultiSZ Kind = 7
)

// Value is a registry value.
type Value struct {
	Kind Kind   `json:"k"`
	S    string `json:"s,omitempty"`
	D    uint32 `json:"d,omitempty"`
}

// Str returns a REG_SZ value.
func Str(s string) Value { return Value{Kind: String, S: s} }

// Multi returns a REG_MULTI_SZ value with one string (Firefox reads JSON this way).
func Multi(s string) Value { return Value{Kind: MultiSZ, S: s} }

// DW returns a REG_DWORD value.
func DW(d uint32) Value { return Value{Kind: DWord, D: d} }

// Bool returns a REG_DWORD 0 or 1.
func Bool(b bool) Value {
	if b {
		return DW(1)
	}
	return DW(0)
}

// Equal reports whether two values have the same type and data.
func (v Value) Equal(o Value) bool { return v.Kind == o.Kind && v.S == o.S && v.D == o.D }

// ErrNotFound means the key or value does not exist.
var ErrNotFound = errors.New("not found")

// Registry is the part of the Windows registry the agent uses. A root is
// "HKLM" or "HKU\<SID>"; a key is a path below it such as
// `Software\Policies\Google\Chrome`.
type Registry interface {
	Get(root, key, name string) (Value, error) // ErrNotFound if missing
	Set(root, key, name string, v Value) error // creates the key if needed
	DeleteValue(root, key, name string) error  // nil if already missing
	ValueNames(root, key string) ([]string, error)
	SubKeys(root, key string) ([]string, error)
	KeyExists(root, key string) bool
	DeleteKeyIfEmpty(root, key string) error // nil if missing or not empty
	RootAvailable(root string) bool          // HKU\<SID> is loaded
}

// Fake is an in-memory Registry for tests.
type Fake struct {
	mu     sync.Mutex
	keys   map[string]map[string]Value // "ROOT\key" (lower case) -> values
	names  map[string]string           // lower -> original key path
	Roots  map[string]bool             // available roots; HKLM always
	Writes int                         // number of Set calls
	Fail   map[string]error            // "root\key\name" -> error on Set
}

// NewFake returns an empty fake registry with HKLM available.
func NewFake(roots ...string) *Fake {
	f := &Fake{keys: map[string]map[string]Value{}, names: map[string]string{}, Roots: map[string]bool{"HKLM": true}}
	for _, r := range roots {
		f.Roots[r] = true
	}
	return f
}

func fk(root, key string) string { return strings.ToLower(root + `\` + strings.Trim(key, `\`)) }

// Get implements Registry.
func (f *Fake) Get(root, key, name string) (Value, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	k, ok := f.keys[fk(root, key)]
	if !ok {
		return Value{}, ErrNotFound
	}
	for n, v := range k {
		if strings.EqualFold(n, name) {
			return v, nil
		}
	}
	return Value{}, ErrNotFound
}

// Set implements Registry.
func (f *Fake) Set(root, key, name string, v Value) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if !f.Roots[root] {
		return errors.New("root not loaded")
	}
	if err := f.Fail[root+`\`+key+`\`+name]; err != nil {
		return err
	}
	f.Writes++
	// create every parent key, as the real registry does
	parts := strings.Split(strings.Trim(key, `\`), `\`)
	for i := 1; i <= len(parts); i++ {
		p := strings.Join(parts[:i], `\`)
		if _, ok := f.keys[fk(root, p)]; !ok {
			f.keys[fk(root, p)] = map[string]Value{}
			f.names[fk(root, p)] = root + `\` + p
		}
	}
	k := f.keys[fk(root, key)]
	for n := range k {
		if strings.EqualFold(n, name) {
			delete(k, n)
		}
	}
	k[name] = v
	return nil
}

// DeleteValue implements Registry.
func (f *Fake) DeleteValue(root, key, name string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if k, ok := f.keys[fk(root, key)]; ok {
		for n := range k {
			if strings.EqualFold(n, name) {
				delete(k, n)
			}
		}
	}
	return nil
}

// ValueNames implements Registry.
func (f *Fake) ValueNames(root, key string) ([]string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	k, ok := f.keys[fk(root, key)]
	if !ok {
		return nil, ErrNotFound
	}
	out := make([]string, 0, len(k))
	for n := range k {
		out = append(out, n)
	}
	sort.Strings(out)
	return out, nil
}

// SubKeys implements Registry.
func (f *Fake) SubKeys(root, key string) ([]string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	pre := fk(root, key) + `\`
	if _, ok := f.keys[fk(root, key)]; !ok {
		return nil, ErrNotFound
	}
	var out []string
	for k := range f.keys {
		if strings.HasPrefix(k, pre) && !strings.Contains(k[len(pre):], `\`) {
			orig := f.names[k]
			out = append(out, orig[strings.LastIndex(orig, `\`)+1:])
		}
	}
	sort.Strings(out)
	return out, nil
}

// KeyExists implements Registry.
func (f *Fake) KeyExists(root, key string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	_, ok := f.keys[fk(root, key)]
	return ok
}

// DeleteKeyIfEmpty implements Registry.
func (f *Fake) DeleteKeyIfEmpty(root, key string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	k, ok := f.keys[fk(root, key)]
	if !ok || len(k) > 0 {
		return nil
	}
	pre := fk(root, key) + `\`
	for o := range f.keys {
		if strings.HasPrefix(o, pre) {
			return nil
		}
	}
	delete(f.keys, fk(root, key))
	delete(f.names, fk(root, key))
	return nil
}

// RootAvailable implements Registry.
func (f *Fake) RootAvailable(root string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.Roots[root]
}

// Dump returns every value as "ROOT\key\name" -> value, for tests.
func (f *Fake) Dump() map[string]Value {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := map[string]Value{}
	for k, vals := range f.keys {
		for n, v := range vals {
			out[f.names[k]+`\`+n] = v
		}
	}
	return out
}

// KeyCount returns the number of keys, for tests.
func (f *Fake) KeyCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.keys)
}
