//go:build !windows

package linuxsys

import (
	"os"
	"testing"
)

// linux/raisesteady-guard.service (installed by the packages) is UnitText.
func TestUnitFileMatches(t *testing.T) {
	want := UnitText("/usr/lib/raisesteady-guard/raisesteady-guard")
	if os.Getenv("WRITE_UNIT") == "1" {
		if err := os.WriteFile("../../../linux/raisesteady-guard.service", []byte(want), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	b, err := os.ReadFile("../../../linux/raisesteady-guard.service")
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != want {
		t.Error("linux/raisesteady-guard.service differs from UnitText: run WRITE_UNIT=1 go test ./internal/linuxsys")
	}
}
