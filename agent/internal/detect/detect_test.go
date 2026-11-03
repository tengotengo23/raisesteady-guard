package detect

import (
	"strings"
	"testing"

	"raisesteady-guard/agent/internal/core"
	"raisesteady-guard/agent/internal/policy"
)

type fakeEnv struct {
	reg   *policy.Fake
	files map[string]string // path (lower) -> product
}

func (f fakeEnv) Reg() policy.Registry { return f.reg }
func (f fakeEnv) ProgramDirs() []string {
	return []string{`C:\Program Files`, `C:\Program Files (x86)`}
}
func (f fakeEnv) Profiles() map[string]string {
	return map[string]string{"S-1-5-21-1-2-3-1001": `C:\Users\kid`}
}
func (f fakeEnv) Exists(p string) bool        { _, ok := f.files[strings.ToLower(p)]; return ok }
func (f fakeEnv) ProductName(p string) string { return f.files[strings.ToLower(p)] }

func TestDetect(t *testing.T) {
	d, err := core.LoadData()
	if err != nil {
		t.Fatal(err)
	}
	reg := policy.NewFake(`HKU\S-1-5-21-1-2-3-1001`)
	_ = reg.Set("HKLM", `SOFTWARE\Clients\StartMenuInternet\Firefox-308046B0AF4A39CB\shell\open\command`, "", policy.Str(`"D:\Apps\Firefox\firefox.exe" -osint -url "%1"`))
	_ = reg.Set(`HKU\S-1-5-21-1-2-3-1001`, `Software\Clients\StartMenuInternet\OperaStable\shell\open\command`, "", policy.Str(`"C:\Users\kid\AppData\Local\Programs\Opera\opera.exe" --single-argument %1`))
	env := fakeEnv{reg: reg, files: map[string]string{
		strings.ToLower(`C:\Program Files\Google\Chrome\Application\chrome.exe`):        "Google Chrome",
		strings.ToLower(`C:\Program Files (x86)\Microsoft\Edge\Application\msedge.exe`): "Microsoft Edge",
		strings.ToLower(`D:\Apps\Firefox\firefox.exe`):                                  "Firefox",
		strings.ToLower(`C:\Users\kid\AppData\Local\Programs\Opera\opera.exe`):          "Opera Internet Browser",
		strings.ToLower(`C:\Users\kid\Desktop\Tor Browser\Browser\firefox.exe`):         "Firefox",
	}}
	got := map[string]Found{}
	for _, f := range Detect(env, d.Unsupported) {
		got[f.ID] = f
	}
	for id, sup := range map[string]bool{"chrome": true, "edge": true, "firefox": true, "opera": false, "tor": false} {
		f, ok := got[id]
		if !ok || f.Supported != sup {
			t.Errorf("%s: found=%v supported=%v", id, ok, f.Supported)
		}
	}
	if len(got) != 5 {
		t.Errorf("found %d: %+v", len(got), got)
	}
	if got["opera"].ProfileSID != "S-1-5-21-1-2-3-1001" {
		t.Error("Opera is in the kid's profile")
	}
}

func TestExeFromCommand(t *testing.T) {
	for in, want := range map[string]string{
		`"C:\Program Files\X\x.exe" --a "%1"`: `C:\Program Files\X\x.exe`,
		`C:\Apps\y.exe -url %1`:               `C:\Apps\y.exe`,
	} {
		if got := ExeFromCommand(in); got != want {
			t.Errorf("%q -> %q", in, got)
		}
	}
}
