package util

import (
	"github.com/alexflint/go-filemutex"
	"github.com/gruntwork-io/go-commons/errors"
)

type Lockfile struct {
	mutex    *filemutex.FileMutex
	filename string
}

func NewLockfile(filename string) *Lockfile {
	return &Lockfile{
		filename: filename,
	}
}

func (lockfile *Lockfile) Unlock() error {
	if err := lockfile.mutex.Unlock(); err != nil {
		return errors.WithStackTrace(err)
	}
	return nil
}

func (lockfile *Lockfile) TryLock() error {
	mutex, err := filemutex.New(lockfile.filename)
	if err != nil {
		return errors.WithStackTrace(err)
	}
	lockfile.mutex = mutex

	if err := mutex.Lock(); err != nil {
		return errors.WithStackTrace(err)
	}
	return nil
}

func AcquireLockfile(filename string) (*Lockfile, error) {
	lockfile := NewLockfile(filename)
	err := lockfile.TryLock()
	return lockfile, err
}
