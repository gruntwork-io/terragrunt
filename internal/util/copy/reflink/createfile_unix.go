//go:build darwin || linux

package reflink

import (
	"os"
	"path/filepath"

	"golang.org/x/sys/unix"
)

func createFile(dir *os.File, name string, perms os.FileMode) (*os.File, error) {
	dirFD := int(dir.Fd())
	fd, err := unix.Openat(dirFD, name, unix.O_WRONLY|unix.O_CREAT|unix.O_EXCL|unix.O_NONBLOCK, uint32(perms))
	if err != nil {
		return nil, err
	}
	fileName := filepath.Join(dir.Name(), name)
	return os.NewFile(uintptr(fd), fileName), nil
}
