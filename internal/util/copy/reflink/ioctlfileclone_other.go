//go:build !linux

package reflink

import (
	"os"
)

func ioctlFileClone(from *os.File, toDir *os.File, toName string) (*os.File, error) {
	return nil, ErrNotOnPlatform
}
