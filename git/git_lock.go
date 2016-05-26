package git

import "fmt"

const TMP_DIR_BASE = "/tmp/terragrunt"

type GitLock struct {
	StateFileId 	string
	LockBranch	string
	RemoteName 	string
}

func (gitLock GitLock) AcquireLock() error {
	// Clone repo into tmp dir
	// Check out LockBranch in tmp dir; create branch if it doesn't exit
	// Git pull latest data
	// Create a lock file named StateFileId.lock; put username, IP, and timestamp in file
	// Commit and push the file to Git
	// If it fails, retry every 30 seconds: delete the file, uncommit, pull, commit, push until success or CTRL+C
	// TODO: on CTRL+C, clean up file?
	return fmt.Errorf("AcquireLock not yet implemented for Git")
}

func (gitLock GitLock) ReleaseLock() error {
	// Delete StateFileId.lock
	// Commit and push changes to Git
	// Delete tmp dir
	return fmt.Errorf("ReleaseLock not yet implemented for Git")
}

func (gitLock GitLock) String() string {
	return fmt.Sprintf("Git lock for state file %s", gitLock.StateFileId)
}
