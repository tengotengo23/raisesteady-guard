//go:build windows

package update

import "golang.org/x/sys/windows"

// adminOnlySDDL: SYSTEM and Administrators only, protected (no inherited
// rights from ProgramData), passed on to files and folders inside.
const adminOnlySDDL = "D:P(A;OICI;FA;;;SY)(A;OICI;FA;;;BA)"

// secureDir sets the admin-only rights on the updates folder itself, so a
// standard user (the child) can never create or swap a file in it, even if
// the data folder's own rights were ever loosened.
func secureDir(dir string) error {
	sd, err := windows.SecurityDescriptorFromString(adminOnlySDDL)
	if err != nil {
		return err
	}
	dacl, _, err := sd.DACL()
	if err != nil {
		return err
	}
	return windows.SetNamedSecurityInfo(dir, windows.SE_FILE_OBJECT,
		windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION, nil, nil, dacl, nil)
}
