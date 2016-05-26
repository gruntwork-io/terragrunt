package git

import "fmt"

type GitLock struct {
	StateFileId 	string
	RemoteName 	string
}

func (gitLock GitLock) AcquireLock() error {
	return fmt.Errorf("AcquireLock not yet implemented for Git")
}

func (gitLock GitLock) ReleaseLock() error {
	return fmt.Errorf("ReleaseLock not yet implemented for Git")
}

func (gitLock GitLock) String() string {
	return fmt.Sprintf("Git lock for state file %s", gitLock.StateFileId)
}
