//go:build !darwin

package reflink

import (
	"os"
)

func clonefile(from *os.File, toDir *os.File, toName string) error {
	return ErrNotOnPlatform
}
