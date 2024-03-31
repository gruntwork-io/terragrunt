package util

import (
	"os"

	"github.com/gruntwork-io/go-commons/errors"
)

type Lockfile struct {
	filename string
	fd       *os.File
}

func NewLockfile(filename string) *Lockfile {
	return &Lockfile{
		filename: filename,
	}
}

func (lockfile *Lockfile) Unlock() error {
	lockfile.fd.Close()
	os.Remove(lockfile.filename)
	return nil
}

func (lockfile *Lockfile) TryLock() error {
	fd, err := os.OpenFile(lockfile.filename, os.O_RDONLY|os.O_CREATE|os.O_EXCL, 0666)
	if err != nil {
		return errors.WithStackTrace(err)
	}
	lockfile.fd = fd

	return nil
}

func AcquireLockfile(filename string) (*Lockfile, error) {
	lockfile := NewLockfile(filename)
	err := lockfile.TryLock()
	return lockfile, err
}
