//go:build windows

// Package winsys is the Windows side of the agent: registry, processes,
// sessions, accounts, profiles and the service. Adapted from the patterns in
// golang.org/x/sys/windows (svc/example, registry), BSD-3-Clause.
package winsys

import (
	"errors"
	"fmt"
	"strings"
	"sync"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"

	"raisesteady-guard/agent/internal/policy"
)

// Registry is the real registry. Roots: "HKLM" or "HKU\<name>".
type Registry struct{}

const view = registry.WOW64_64KEY

func split(root, key string) (registry.Key, string, string, error) {
	key = strings.Trim(key, `\`)
	switch {
	case root == "HKLM":
		return registry.LOCAL_MACHINE, key, "", nil
	case strings.HasPrefix(root, `HKU\`):
		sub := strings.TrimPrefix(root, `HKU\`)
		if sub == "" || strings.Contains(sub, `\`) {
			return 0, "", "", errors.New("bad root")
		}
		p := sub
		if key != "" {
			p += `\` + key
		}
		return registry.USERS, p, strings.TrimPrefix(sub, "KG_"), nil
	}
	return 0, "", "", errors.New("bad root")
}

func notFound(err error) bool {
	return errors.Is(err, registry.ErrNotExist) || errors.Is(err, windows.ERROR_FILE_NOT_FOUND) || errors.Is(err, windows.ERROR_PATH_NOT_FOUND)
}

// Get implements policy.Registry.
func (Registry) Get(root, key, name string) (policy.Value, error) {
	base, p, _, err := split(root, key)
	if err != nil {
		return policy.Value{}, err
	}
	k, err := registry.OpenKey(base, p, registry.QUERY_VALUE|view)
	if err != nil {
		if notFound(err) {
			return policy.Value{}, policy.ErrNotFound
		}
		return policy.Value{}, err
	}
	defer k.Close()
	_, typ, err := k.GetValue(name, nil)
	if err != nil {
		if notFound(err) {
			return policy.Value{}, policy.ErrNotFound
		}
		return policy.Value{}, err
	}
	switch typ {
	case registry.SZ, registry.EXPAND_SZ:
		s, _, err := k.GetStringValue(name)
		if err != nil {
			return policy.Value{}, err
		}
		return policy.Str(s), nil
	case registry.MULTI_SZ:
		l, _, err := k.GetStringsValue(name)
		if err != nil {
			return policy.Value{}, err
		}
		return policy.Multi(strings.Join(l, "\n")), nil
	case registry.DWORD:
		d, _, err := k.GetIntegerValue(name)
		if err != nil {
			return policy.Value{}, err
		}
		return policy.DW(uint32(d)), nil
	}
	// Another type: never equal to ours, so Apply rewrites it.
	return policy.Value{Kind: policy.Kind(100 + typ)}, nil
}

// userSDDL is the ACL of every key the agent writes in a user's hive: SYSTEM
// and Administrators full control, the user and restricted code read only,
// inheritance from the parent off.
func userSDDL(sid string) string {
	return "D:P(A;CI;KA;;;SY)(A;CI;KA;;;BA)(A;CI;KR;;;" + sid + ")(A;CI;KR;;;RC)"
}

var privOnce sync.Once

// protect makes SYSTEM the owner of a key and sets userSDDL. Setting the owner
// first (with SeTakeOwnership and SeRestore) works even if someone gave the key
// an ACL that shuts SYSTEM out; the owner may then always rewrite the ACL.
func protect(name, sid string) error {
	privOnce.Do(func() { _ = enablePrivileges("SeTakeOwnershipPrivilege", "SeRestorePrivilege") })
	system, err := windows.CreateWellKnownSid(windows.WinLocalSystemSid)
	if err != nil {
		return err
	}
	if err := windows.SetNamedSecurityInfo(name, windows.SE_REGISTRY_KEY, windows.OWNER_SECURITY_INFORMATION, system, nil, nil, nil); err != nil {
		return fmt.Errorf("owner: %w", err)
	}
	sd, err := windows.SecurityDescriptorFromString(userSDDL(sid))
	if err != nil {
		return err
	}
	dacl, _, err := sd.DACL()
	if err != nil {
		return err
	}
	if err := windows.SetNamedSecurityInfo(name, windows.SE_REGISTRY_KEY,
		windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION, nil, nil, dacl, nil); err != nil {
		return fmt.Errorf("acl: %w", err)
	}
	return nil
}

// ensureKey opens the key, creating it if needed. In a user's hive every key
// from Software\Policies\<Vendor> down to the key written gets the protected
// ACL (keys that existed before us too), so the user can neither change nor
// delete the policies. The one exception is a Software\Policies\Microsoft key
// that Windows made: its ACL is left as Windows set it.
func ensureKey(base registry.Key, p, sid string) (registry.Key, error) {
	if sid == "" {
		k, _, err := registry.CreateKey(base, p, registry.ALL_ACCESS|view)
		return k, err
	}
	parts := strings.Split(p, `\`)
	// parts[0] is the hive; parts[1:3] are Software\Policies.
	for i := 2; i <= len(parts); i++ {
		cur := strings.Join(parts[:i], `\`)
		depth := i - 3 // 0 = Software\Policies, 1 = vendor, 2 = product
		existed := false
		if k, err := registry.OpenKey(base, cur, registry.QUERY_VALUE|view); err == nil {
			k.Close()
			existed = true
		}
		// Software\Policies itself only when we create it (it normally exists,
		// read-only for the user).
		guarded := underPolicies(cur) && ((depth >= 1 && !(existed && depth == 1 && strings.EqualFold(parts[3], "Microsoft"))) ||
			(depth == 0 && !existed))
		if existed && guarded {
			if err := protect(`USERS\`+cur, sid); err != nil {
				return 0, err
			}
		}
		k, _, err := registry.CreateKey(base, cur, registry.ALL_ACCESS|view)
		if err != nil {
			return 0, err
		}
		if !existed && guarded {
			if err := protect(`USERS\`+cur, sid); err != nil {
				k.Close()
				return 0, err
			}
		}
		if i == len(parts) {
			return k, nil
		}
		k.Close()
	}
	return 0, errors.New("empty key path")
}

func underPolicies(p string) bool {
	// p is "<hive>\Software\Policies[\...]"
	i := strings.Index(p, `\`)
	if i < 0 {
		return false
	}
	rest := strings.ToLower(p[i+1:])
	return rest == `software\policies` || strings.HasPrefix(rest, `software\policies\`)
}

// Set implements policy.Registry.
func (Registry) Set(root, key, name string, v policy.Value) error {
	base, p, sid, err := split(root, key)
	if err != nil {
		return err
	}
	k, err := ensureKey(base, p, sid)
	if err != nil {
		return err
	}
	defer k.Close()
	switch v.Kind {
	case policy.String:
		return k.SetStringValue(name, v.S)
	case policy.DWord:
		return k.SetDWordValue(name, v.D)
	case policy.MultiSZ:
		return k.SetStringsValue(name, strings.Split(v.S, "\n"))
	}
	return errors.New("unsupported value kind")
}

// DeleteValue implements policy.Registry.
func (Registry) DeleteValue(root, key, name string) error {
	base, p, _, err := split(root, key)
	if err != nil {
		return err
	}
	k, err := registry.OpenKey(base, p, registry.SET_VALUE|view)
	if err != nil {
		if notFound(err) {
			return nil
		}
		return err
	}
	defer k.Close()
	if err := k.DeleteValue(name); err != nil && !notFound(err) {
		return err
	}
	return nil
}

// ValueNames implements policy.Registry.
func (Registry) ValueNames(root, key string) ([]string, error) {
	base, p, _, err := split(root, key)
	if err != nil {
		return nil, err
	}
	k, err := registry.OpenKey(base, p, registry.QUERY_VALUE|view)
	if err != nil {
		if notFound(err) {
			return nil, policy.ErrNotFound
		}
		return nil, err
	}
	defer k.Close()
	return k.ReadValueNames(0)
}

// SubKeys implements policy.Registry.
func (Registry) SubKeys(root, key string) ([]string, error) {
	base, p, _, err := split(root, key)
	if err != nil {
		return nil, err
	}
	k, err := registry.OpenKey(base, p, registry.ENUMERATE_SUB_KEYS|view)
	if err != nil {
		if notFound(err) {
			return nil, policy.ErrNotFound
		}
		return nil, err
	}
	defer k.Close()
	return k.ReadSubKeyNames(0)
}

// KeyExists implements policy.Registry.
func (Registry) KeyExists(root, key string) bool {
	base, p, _, err := split(root, key)
	if err != nil {
		return false
	}
	k, err := registry.OpenKey(base, p, registry.QUERY_VALUE|view)
	if err != nil {
		return false
	}
	k.Close()
	return true
}

// DeleteKeyIfEmpty implements policy.Registry.
func (Registry) DeleteKeyIfEmpty(root, key string) error {
	base, p, _, err := split(root, key)
	if err != nil {
		return err
	}
	k, err := registry.OpenKey(base, p, registry.QUERY_VALUE|view)
	if err != nil {
		return nil
	}
	info, err := k.Stat()
	k.Close()
	if err != nil || info.SubKeyCount > 0 || info.ValueCount > 0 {
		return nil
	}
	if err := registry.DeleteKey(base, p); err != nil && !notFound(err) {
		return err
	}
	return nil
}

// RootAvailable implements policy.Registry.
func (Registry) RootAvailable(root string) bool {
	if root == "HKLM" {
		return true
	}
	base, p, _, err := split(root, "")
	if err != nil {
		return false
	}
	k, err := registry.OpenKey(base, p, registry.QUERY_VALUE|view)
	if err != nil {
		return false
	}
	k.Close()
	return true
}
