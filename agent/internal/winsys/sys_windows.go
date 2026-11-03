//go:build windows

package winsys

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"

	"raisesteady-guard/agent/internal/api"
	"raisesteady-guard/agent/internal/core"
	"raisesteady-guard/agent/internal/detect"
	"raisesteady-guard/agent/internal/guard"
	"raisesteady-guard/agent/internal/policy"
)

var (
	advapi32         = windows.NewLazySystemDLL("advapi32.dll")
	procRegLoadKey   = advapi32.NewProc("RegLoadKeyW")
	procRegUnLoadKey = advapi32.NewProc("RegUnLoadKeyW")
	wtsapi32         = windows.NewLazySystemDLL("wtsapi32.dll")
	procWTSSendMsg   = wtsapi32.NewProc("WTSSendMessageW")
)

// Sys implements agent.System on Windows.
type Sys struct {
	mu sync.Mutex
}

// Reg implements agent.System.
func (*Sys) Reg() policy.Registry { return Registry{} }

const profileList = `SOFTWARE\Microsoft\Windows NT\CurrentVersion\ProfileList`

func profileDir(sid string) (string, error) {
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, profileList+`\`+sid, registry.QUERY_VALUE|view)
	if err != nil {
		return "", core.ErrNoProfile
	}
	defer k.Close()
	p, _, err := k.GetStringValue("ProfileImagePath")
	if err != nil {
		return "", core.ErrNoProfile
	}
	if x, err := registry.ExpandString(p); err == nil {
		p = x
	}
	return p, nil
}

func enablePrivileges(names ...string) error {
	var tok windows.Token
	if err := windows.OpenProcessToken(windows.CurrentProcess(), windows.TOKEN_ADJUST_PRIVILEGES|windows.TOKEN_QUERY, &tok); err != nil {
		return err
	}
	defer tok.Close()
	for _, n := range names {
		var luid windows.LUID
		if err := windows.LookupPrivilegeValue(nil, windows.StringToUTF16Ptr(n), &luid); err != nil {
			return err
		}
		tp := windows.Tokenprivileges{PrivilegeCount: 1}
		tp.Privileges[0] = windows.LUIDAndAttributes{Luid: luid, Attributes: windows.SE_PRIVILEGE_ENABLED}
		if err := windows.AdjustTokenPrivileges(tok, false, &tp, 0, nil, nil); err != nil {
			return err
		}
	}
	return nil
}

// MountHive implements agent.System: it loads a signed-out user's hive under
// HKU\KG_<SID> for the length of one apply.
func (s *Sys) MountHive(sid string) (string, func() error, error) {
	dir, err := profileDir(sid)
	if err != nil {
		return "", nil, err
	}
	file := filepath.Join(dir, "NTUSER.DAT")
	if _, err := os.Stat(file); err != nil {
		return "", nil, core.ErrNoProfile
	}
	if err := enablePrivileges("SeRestorePrivilege", "SeBackupPrivilege"); err != nil {
		return "", nil, fmt.Errorf("privileges: %w", err)
	}
	name := "KG_" + sid
	n16, _ := windows.UTF16PtrFromString(name)
	f16, _ := windows.UTF16PtrFromString(file)
	r, _, _ := procRegLoadKey.Call(uintptr(windows.HKEY_USERS), uintptr(unsafe.Pointer(n16)), uintptr(unsafe.Pointer(f16)))
	if r != 0 {
		return "", nil, fmt.Errorf("load hive: %w", windows.Errno(r))
	}
	unmount := func() error {
		r, _, _ := procRegUnLoadKey.Call(uintptr(windows.HKEY_USERS), uintptr(unsafe.Pointer(n16)))
		if r != 0 {
			return windows.Errno(r)
		}
		return nil
	}
	return `HKU\` + name, unmount, nil
}

// SignedIn implements agent.System: the user's hive is loaded.
func (*Sys) SignedIn(sid string) bool { return Registry{}.RootAvailable(`HKU\` + sid) }

type userInfo1 struct {
	Name        *uint16
	Password    *uint16
	PasswordAge uint32
	Priv        uint32
	HomeDir     *uint16
	Comment     *uint16
	Flags       uint32
	ScriptPath  *uint16
}

const (
	filterNormalAccount = 0x0002
	ufAccountDisable    = 0x0002
	userPrivAdmin       = 2
)

// LocalAccounts implements agent.System: enabled local accounts, without the
// built-in Administrator, Guest, DefaultAccount and WDAGUtilityAccount.
func (*Sys) LocalAccounts() ([]api.LocalAccount, error) {
	var buf *byte
	var read, total, resume uint32
	if err := windows.NetUserEnum(nil, 1, filterNormalAccount, &buf, 0xFFFFFFFF, &read, &total, &resume); err != nil {
		return nil, err
	}
	defer windows.NetApiBufferFree(buf)
	users := unsafe.Slice((*userInfo1)(unsafe.Pointer(buf)), read)
	var out []api.LocalAccount
	for _, u := range users {
		if u.Flags&ufAccountDisable != 0 {
			continue
		}
		name := windows.UTF16PtrToString(u.Name)
		sid, _, _, err := windows.LookupSID("", name)
		if err != nil {
			continue
		}
		ss := sid.String()
		rid := ss[strings.LastIndex(ss, "-")+1:]
		if rid == "500" || rid == "501" || rid == "503" || rid == "504" {
			continue
		}
		display := name
		var b10 *byte
		if windows.NetUserGetInfo(nil, u.Name, 10, &b10) == nil {
			if full := windows.UTF16PtrToString((*windows.UserInfo10)(unsafe.Pointer(b10)).FullName); full != "" && full != name {
				display = full + " (" + name + ")"
			}
			windows.NetApiBufferFree(b10)
		}
		out = append(out, api.LocalAccount{SID: ss, Name: display, Admin: u.Priv == userPrivAdmin})
	}
	return out, nil
}

// UserLang implements agent.System: the user's Windows display language.
func (*Sys) UserLang(sid string) string {
	k, err := registry.OpenKey(registry.USERS, sid+`\Control Panel\Desktop`, registry.QUERY_VALUE)
	if err == nil {
		l, _, err2 := k.GetStringsValue("PreferredUILanguages")
		k.Close()
		if err2 == nil && len(l) > 0 {
			return guard.LangFromLocale(l[0])
		}
	}
	k, err = registry.OpenKey(registry.USERS, sid+`\Control Panel\International`, registry.QUERY_VALUE)
	if err != nil {
		return ""
	}
	defer k.Close()
	if l, _, err := k.GetStringValue("LocaleName"); err == nil {
		return guard.LangFromLocale(l)
	}
	return ""
}

// SystemLang implements agent.System.
func (*Sys) SystemLang() string {
	l, err := windows.GetSystemPreferredUILanguages(windows.MUI_LANGUAGE_NAME)
	if err != nil || len(l) == 0 {
		return "en"
	}
	return guard.LangFromLocale(l[0])
}

// SecureDir implements agent.System: SYSTEM and Administrators only, inherited by files.
func (*Sys) SecureDir(path string) error {
	sd, err := windows.SecurityDescriptorFromString("D:P(A;OICI;FA;;;SY)(A;OICI;FA;;;BA)")
	if err != nil {
		return err
	}
	dacl, _, err := sd.DACL()
	if err != nil {
		return err
	}
	return windows.SetNamedSecurityInfo(path, windows.SE_FILE_OBJECT,
		windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION, nil, nil, dacl, nil)
}

// DetectEnv implements agent.System.
func (s *Sys) DetectEnv() detect.Env { return env{} }

// GuardSys implements agent.System.
func (s *Sys) GuardSys() guard.System { return &procs{} }

type env struct{}

func (env) Reg() policy.Registry { return Registry{} }

func (env) ProgramDirs() []string {
	set := map[string]bool{}
	var out []string
	for _, v := range []string{"ProgramW6432", "ProgramFiles", "ProgramFiles(x86)"} {
		if d := os.Getenv(v); d != "" && !set[strings.ToLower(d)] {
			set[strings.ToLower(d)] = true
			out = append(out, d)
		}
	}
	if len(out) == 0 {
		out = []string{`C:\Program Files`, `C:\Program Files (x86)`}
	}
	return out
}

func (env) Profiles() map[string]string {
	out := map[string]string{}
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, profileList, registry.ENUMERATE_SUB_KEYS|view)
	if err != nil {
		return out
	}
	defer k.Close()
	subs, _ := k.ReadSubKeyNames(0)
	for _, sid := range subs {
		if !strings.HasPrefix(sid, "S-1-5-21-") && !strings.HasPrefix(sid, "S-1-12-1-") {
			continue
		}
		if d, err := profileDir(sid); err == nil {
			out[sid] = d
		}
	}
	return out
}

func (env) Exists(path string) bool {
	st, err := os.Stat(path)
	return err == nil && !st.IsDir()
}

func (env) ProductName(path string) string { return productName(path) }

// productName reads ProductName from a program's version info.
func productName(path string) string {
	size, err := windows.GetFileVersionInfoSize(path, nil)
	if err != nil || size == 0 || size > 1<<22 {
		return ""
	}
	buf := make([]byte, size)
	if err := windows.GetFileVersionInfo(path, 0, size, unsafe.Pointer(&buf[0])); err != nil {
		return ""
	}
	var tr *[2]uint16
	var n uint32
	cps := []string{}
	if windows.VerQueryValue(unsafe.Pointer(&buf[0]), `\VarFileInfo\Translation`, unsafe.Pointer(&tr), &n) == nil && n >= 4 {
		cps = append(cps, fmt.Sprintf("%04x%04x", tr[0], tr[1]))
	}
	cps = append(cps, "040904b0", "040904e4", "000004b0")
	for _, cp := range cps {
		var p *uint16
		var l uint32
		if windows.VerQueryValue(unsafe.Pointer(&buf[0]), `\StringFileInfo\`+cp+`\ProductName`, unsafe.Pointer(&p), &l) == nil && l > 0 {
			return strings.TrimSpace(windows.UTF16PtrToString(p))
		}
	}
	return ""
}

// procs implements guard.System.
type procs struct{}

func (*procs) Processes() ([]guard.Proc, error) {
	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return nil, err
	}
	defer windows.CloseHandle(snap)
	var e windows.ProcessEntry32
	e.Size = uint32(unsafe.Sizeof(e))
	var out []guard.Proc
	for err = windows.Process32First(snap, &e); err == nil; err = windows.Process32Next(snap, &e) {
		var sess uint32
		if windows.ProcessIdToSessionId(e.ProcessID, &sess) != nil || sess == 0 {
			continue
		}
		p := guard.Proc{PID: e.ProcessID, Session: sess}
		h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, e.ProcessID)
		if err != nil {
			continue
		}
		buf := make([]uint16, windows.MAX_LONG_PATH)
		n := uint32(len(buf))
		if windows.QueryFullProcessImageName(h, 0, &buf[0], &n) == nil {
			p.Path = windows.UTF16ToString(buf[:n])
		}
		var tok windows.Token
		if windows.OpenProcessToken(h, windows.TOKEN_QUERY, &tok) == nil {
			if u, err := tok.GetTokenUser(); err == nil {
				p.SID = u.User.Sid.String()
			}
			tok.Close()
		}
		windows.CloseHandle(h)
		if p.Path != "" && p.SID != "" {
			out = append(out, p)
		}
	}
	return out, nil
}

func (*procs) ProductName(path string) string { return productName(path) }

func (*procs) Kill(pid uint32) error {
	h, err := windows.OpenProcess(windows.PROCESS_TERMINATE, false, pid)
	if err != nil {
		return err
	}
	defer windows.CloseHandle(h)
	return windows.TerminateProcess(h, 1)
}

const (
	mbOK            = 0x0
	mbIconInfo      = 0x40
	mbSetForeground = 0x10000
	mbTopmost       = 0x40000
)

// Message shows a message box in a session without waiting for it.
func (*procs) Message(session uint32, title, text string) error {
	t16, _ := windows.UTF16FromString(title)
	m16, _ := windows.UTF16FromString(text)
	var resp uint32
	r, _, err := procWTSSendMsg.Call(0, uintptr(session),
		uintptr(unsafe.Pointer(&t16[0])), uintptr((len(t16)-1)*2),
		uintptr(unsafe.Pointer(&m16[0])), uintptr((len(m16)-1)*2),
		mbOK|mbIconInfo|mbSetForeground|mbTopmost, 30, uintptr(unsafe.Pointer(&resp)), 0)
	if r == 0 {
		return err
	}
	return nil
}

func (s *procs) UserLang(sid string) string {
	if l := (&Sys{}).UserLang(sid); l != "" {
		return l
	}
	return "en"
}

// SessionSID returns the SID of the user signed in to a session (service only).
func SessionSID(session uint32) (string, error) {
	var tok windows.Token
	if err := windows.WTSQueryUserToken(session, &tok); err != nil {
		return "", err
	}
	defer tok.Close()
	u, err := tok.GetTokenUser()
	if err != nil {
		return "", err
	}
	return u.User.Sid.String(), nil
}

// IsAdmin reports whether this process runs elevated.
func IsAdmin() bool { return windows.GetCurrentProcessToken().IsElevated() }
