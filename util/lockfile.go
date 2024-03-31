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
	return os.Remove(lockfile.filename)
}

func (lockfile *Lockfile) TryLock() error {
	if FileExists(lockfile.filename) {
		return errors.Errorf("file already locked")
	}

	file, err := os.Create(lockfile.filename)
	if err != nil {
		return errors.WithStackTrace(err)
	}
	defer file.Close() //nolint:errcheck

	return nil
}

func AcquireLockfile(filename string) (*Lockfile, error) {
	lockfile := NewLockfile(filename)
	err := lockfile.TryLock()
	return lockfile, err
}
