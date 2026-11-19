//go:build !windows && !linux

package update

import (
	"context"
	"errors"
)

func platformTarget() (key, kind string, err error) {
	return "", "", errors.New("no updates on this system")
}

func platformInstall(context.Context, string, string) error {
	return errors.New("no updates on this system")
}
