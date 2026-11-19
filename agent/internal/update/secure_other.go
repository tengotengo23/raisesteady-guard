//go:build !windows

package update

import "os"

// secureDir makes the updates folder root-only (0700), whatever the umask.
func secureDir(dir string) error { return os.Chmod(dir, 0o700) }
