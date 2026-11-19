//go:build windows

package update

import (
	"strings"
	"testing"

	"golang.org/x/sys/windows"
)

// checkAdminOnly: on Windows the folder's mode bits say nothing; read its ACL.
// Windows adds the auto-inherited flag (AI) by itself when it applies a DACL to a folder,
// so the text differs from what we set ("D:PAI" for "D:P"). The check compares what matters:
// the DACL is protected (nothing inherited from the parent folder) and holds exactly our entries.
func checkAdminOnly(t *testing.T, dir string) {
	t.Helper()
	sd, err := windows.GetNamedSecurityInfo(dir, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		t.Fatalf("updates folder rights: %v", err)
	}
	ctrl, _, err := sd.Control()
	if err != nil {
		t.Fatalf("updates folder rights: %v", err)
	}
	if ctrl&windows.SE_DACL_PROTECTED == 0 {
		t.Fatalf("updates folder rights = %s: not protected from the parent folder", sd.String())
	}
	if got, want := aceList(sd.String()), aceList(adminOnlySDDL); got != want {
		t.Fatalf("updates folder rights = %s, want the entries %s", sd.String(), want)
	}
}

// aceList returns the entries of an SDDL DACL: "D:PAI(A;..)(A;..)" gives "(A;..)(A;..)".
func aceList(sddl string) string {
	if i := strings.IndexByte(sddl, '('); i >= 0 {
		return sddl[i:]
	}
	return ""
}
