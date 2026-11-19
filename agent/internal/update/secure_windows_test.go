//go:build windows

package update

import (
	"testing"

	"golang.org/x/sys/windows"
)

// checkAdminOnly: on Windows the folder's mode bits say nothing; read its ACL.
func checkAdminOnly(t *testing.T, dir string) {
	t.Helper()
	sd, err := windows.GetNamedSecurityInfo(dir, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		t.Fatalf("updates folder rights: %v", err)
	}
	if got := sd.String(); got != adminOnlySDDL {
		t.Fatalf("updates folder rights = %s, want %s", got, adminOnlySDDL)
	}
}
