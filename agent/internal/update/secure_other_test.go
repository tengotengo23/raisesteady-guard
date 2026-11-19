//go:build !windows

package update

import (
	"os"
	"testing"
)

func checkAdminOnly(t *testing.T, dir string) {
	t.Helper()
	fi, err := os.Stat(dir)
	if err != nil || fi.Mode().Perm() != 0o700 {
		t.Fatalf("updates folder: %v %v", fi, err)
	}
}
