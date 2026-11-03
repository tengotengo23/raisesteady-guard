package policy

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// Entry is one desired registry value. For a list policy (List true), Key is
// the list key, the value name is chosen at apply time ("1", "2", ...) and Val
// is the item; other items in the same list are kept.
type Entry struct {
	Root string
	Key  string
	Name string
	Val  Value
	List bool
}

// Record is one value the agent wrote (kept in state.json).
type Record struct {
	Root string `json:"root"`
	Key  string `json:"key"`
	Name string `json:"name"`
	Val  Value  `json:"val"`
	// Prev is the value that was there before us; uninstall puts it back.
	Prev *Value `json:"prev,omitempty"`
	List bool   `json:"list,omitempty"`
	// Adopted marks a list item that existed before us: never removed.
	Adopted bool `json:"adopted,omitempty"`
}

// KeyRef is a registry key the agent created.
type KeyRef struct {
	Root string `json:"root"`
	Key  string `json:"key"`
}

// State is what the agent remembers about its own writes.
type State struct {
	Records     []Record `json:"records"`
	CreatedKeys []KeyRef `json:"createdKeys"`
}

// Result counts what one Apply did.
type Result struct {
	Written   int      `json:"written"`
	Unchanged int      `json:"unchanged"`
	Removed   int      `json:"removed"`
	Pending   int      `json:"pending"` // entries for roots that are not loaded
	Errors    []string `json:"errors,omitempty"`
}

func entryID(root, key, name string, list bool, val Value) string {
	if list {
		return strings.ToLower(root+"|"+key) + "|#" + val.S
	}
	return strings.ToLower(root + "|" + key + "|" + name)
}

func (r Record) id() string { return entryID(r.Root, r.Key, r.Name, r.List, r.Val) }
func (e Entry) id() string  { return entryID(e.Root, e.Key, e.Name, e.List, e.Val) }

// policiesRoot is the boundary below which the agent may create and delete keys.
const policiesRoot = `software\policies\`

func (st *State) trackCreated(reg Registry, root, key string) {
	parts := strings.Split(strings.Trim(key, `\`), `\`)
	for i := 3; i <= len(parts); i++ { // Software\Policies\<Vendor> and deeper
		p := strings.Join(parts[:i], `\`)
		if !strings.HasPrefix(strings.ToLower(p)+`\`, policiesRoot) {
			continue
		}
		if reg.KeyExists(root, p) {
			continue
		}
		known := false
		for _, k := range st.CreatedKeys {
			if strings.EqualFold(k.Root, root) && strings.EqualFold(k.Key, p) {
				known = true
				break
			}
		}
		if !known {
			st.CreatedKeys = append(st.CreatedKeys, KeyRef{Root: root, Key: p})
		}
	}
}

func listNames(reg Registry, root, key string) []string {
	names, err := reg.ValueNames(root, key)
	if err != nil {
		return nil
	}
	return names
}

func findListItem(reg Registry, root, key string, val Value) (string, bool) {
	for _, n := range listNames(reg, root, key) {
		if v, err := reg.Get(root, key, n); err == nil && v.Equal(val) {
			return n, true
		}
	}
	return "", false
}

func freeListName(reg Registry, root, key string) string {
	used := map[string]bool{}
	for _, n := range listNames(reg, root, key) {
		used[n] = true
	}
	for i := 1; ; i++ {
		if !used[strconv.Itoa(i)] {
			return strconv.Itoa(i)
		}
	}
}

// compactList renumbers a list key's numeric values to 1..n in their order,
// because Chromium stops reading a list at the first missing number.
func compactList(reg Registry, root, key string) error {
	type item struct {
		n int
		v Value
	}
	var items []item
	for _, name := range listNames(reg, root, key) {
		n, err := strconv.Atoi(name)
		if err != nil || n < 1 {
			continue
		}
		v, err := reg.Get(root, key, name)
		if err != nil {
			continue
		}
		items = append(items, item{n, v})
	}
	sort.Slice(items, func(i, j int) bool { return items[i].n < items[j].n })
	for i, it := range items {
		want := i + 1
		if it.n == want {
			continue
		}
		if err := reg.Set(root, key, strconv.Itoa(want), it.v); err != nil {
			return err
		}
		if err := reg.DeleteValue(root, key, strconv.Itoa(it.n)); err != nil {
			return err
		}
	}
	return nil
}

// Apply brings the registry to the desired entries: it writes only values that
// differ, records what it wrote, and removes or restores its own earlier
// values that are no longer desired. Roots that are not loaded are left alone
// (their records are kept for later).
func Apply(reg Registry, st *State, desired []Entry) Result {
	var res Result
	recIdx := map[string]int{}
	for i, r := range st.Records {
		recIdx[r.id()] = i
	}
	want := map[string]bool{}
	for _, e := range desired {
		want[e.id()] = true
		if !reg.RootAvailable(e.Root) {
			res.Pending++
			continue
		}
		if e.List {
			applyList(reg, st, recIdx, e, &res)
		} else {
			applyScalar(reg, st, recIdx, e, &res)
		}
	}
	// Remove what we wrote earlier and no longer want.
	var keep []Record
	for _, r := range st.Records {
		if want[r.id()] || !reg.RootAvailable(r.Root) {
			keep = append(keep, r)
			continue
		}
		if err := removeRecord(reg, r); err != nil {
			res.Errors = append(res.Errors, err.Error())
			keep = append(keep, r)
			continue
		}
		res.Removed++
	}
	st.Records = keep
	cleanupKeys(reg, st)
	return res
}

func applyScalar(reg Registry, st *State, recIdx map[string]int, e Entry, res *Result) {
	cur, err := reg.Get(e.Root, e.Key, e.Name)
	exists := err == nil
	i, have := recIdx[e.id()]
	if exists && cur.Equal(e.Val) {
		if !have {
			// Already set to our value by someone else: remember it, and
			// on removal leave it as it was (Prev equals Val).
			c := cur
			st.Records = append(st.Records, Record{Root: e.Root, Key: e.Key, Name: e.Name, Val: e.Val, Prev: &c})
			recIdx[e.id()] = len(st.Records) - 1
		}
		res.Unchanged++
		return
	}
	st.trackCreated(reg, e.Root, e.Key)
	if err := reg.Set(e.Root, e.Key, e.Name, e.Val); err != nil {
		res.Errors = append(res.Errors, fmt.Sprintf("set %s\\%s\\%s: %v", e.Root, e.Key, e.Name, err))
		return
	}
	res.Written++
	if have {
		st.Records[i].Val = e.Val
		return
	}
	r := Record{Root: e.Root, Key: e.Key, Name: e.Name, Val: e.Val}
	if exists {
		c := cur
		r.Prev = &c
	}
	st.Records = append(st.Records, r)
	recIdx[e.id()] = len(st.Records) - 1
}

func applyList(reg Registry, st *State, recIdx map[string]int, e Entry, res *Result) {
	i, have := recIdx[e.id()]
	if name, ok := findListItem(reg, e.Root, e.Key, e.Val); ok {
		if have {
			st.Records[i].Name = name
		} else {
			st.Records = append(st.Records, Record{Root: e.Root, Key: e.Key, Name: name, Val: e.Val, List: true, Adopted: true})
			recIdx[e.id()] = len(st.Records) - 1
		}
		res.Unchanged++
		return
	}
	st.trackCreated(reg, e.Root, e.Key)
	if err := compactList(reg, e.Root, e.Key); err != nil {
		res.Errors = append(res.Errors, fmt.Sprintf("compact %s\\%s: %v", e.Root, e.Key, err))
	}
	name := freeListName(reg, e.Root, e.Key)
	if err := reg.Set(e.Root, e.Key, name, e.Val); err != nil {
		res.Errors = append(res.Errors, fmt.Sprintf("set %s\\%s\\%s: %v", e.Root, e.Key, name, err))
		return
	}
	res.Written++
	if have {
		st.Records[i].Name = name
		st.Records[i].Adopted = false
		return
	}
	st.Records = append(st.Records, Record{Root: e.Root, Key: e.Key, Name: name, Val: e.Val, List: true})
	recIdx[e.id()] = len(st.Records) - 1
}

func removeRecord(reg Registry, r Record) error {
	if r.List {
		if r.Adopted {
			return nil
		}
		name, ok := findListItem(reg, r.Root, r.Key, r.Val)
		if !ok {
			return nil
		}
		if err := reg.DeleteValue(r.Root, r.Key, name); err != nil {
			return err
		}
		return compactList(reg, r.Root, r.Key)
	}
	cur, err := reg.Get(r.Root, r.Key, r.Name)
	if err != nil || !cur.Equal(r.Val) {
		// Gone, or changed by someone else since: not ours any more.
		return nil
	}
	if r.Prev != nil {
		if r.Prev.Equal(r.Val) {
			return nil
		}
		return reg.Set(r.Root, r.Key, r.Name, *r.Prev)
	}
	return reg.DeleteValue(r.Root, r.Key, r.Name)
}

// cleanupKeys deletes the keys we created once they are empty, deepest first.
func cleanupKeys(reg Registry, st *State) {
	sort.SliceStable(st.CreatedKeys, func(i, j int) bool {
		return strings.Count(st.CreatedKeys[i].Key, `\`) > strings.Count(st.CreatedKeys[j].Key, `\`)
	})
	var keep []KeyRef
	for _, k := range st.CreatedKeys {
		if !reg.RootAvailable(k.Root) {
			keep = append(keep, k)
			continue
		}
		_ = reg.DeleteKeyIfEmpty(k.Root, k.Key)
		if reg.KeyExists(k.Root, k.Key) {
			keep = append(keep, k)
		}
	}
	st.CreatedKeys = keep
}

// RemoveAll removes or restores every value the agent wrote on available roots.
func RemoveAll(reg Registry, st *State) Result { return Apply(reg, st, nil) }

// Alias maps logical roots (HKU\<SID>) to the roots where a hive is mounted
// for a moment (HKU\KG_<SID>), so records keep their logical root.
type Alias struct {
	Registry
	Map map[string]string
}

func (a Alias) r(root string) string {
	if p, ok := a.Map[root]; ok {
		return p
	}
	return root
}

func (a Alias) Get(root, key, name string) (Value, error) {
	return a.Registry.Get(a.r(root), key, name)
}
func (a Alias) Set(root, key, name string, v Value) error {
	return a.Registry.Set(a.r(root), key, name, v)
}
func (a Alias) DeleteValue(root, key, name string) error {
	return a.Registry.DeleteValue(a.r(root), key, name)
}
func (a Alias) ValueNames(root, key string) ([]string, error) {
	return a.Registry.ValueNames(a.r(root), key)
}
func (a Alias) SubKeys(root, key string) ([]string, error) { return a.Registry.SubKeys(a.r(root), key) }
func (a Alias) KeyExists(root, key string) bool            { return a.Registry.KeyExists(a.r(root), key) }
func (a Alias) DeleteKeyIfEmpty(root, key string) error {
	return a.Registry.DeleteKeyIfEmpty(a.r(root), key)
}
func (a Alias) RootAvailable(root string) bool { return a.Registry.RootAvailable(a.r(root)) }
