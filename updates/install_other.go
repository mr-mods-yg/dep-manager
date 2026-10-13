//go:build !windows && !linux

package updates

import (
	"context"
	"errors"
)

func installAppUpdate(context.Context, string, string) error {
	return errors.New("automatic installation is not supported on this platform")
}
