package git

import (
	"fmt"
	"io/ioutil"
	"path"
	"encoding/json"
	"os"
	"github.com/gruntwork-io/terragrunt/shell"
	"github.com/gruntwork-io/terragrunt/util"
	"github.com/gruntwork-io/terragrunt/locks"
	"strings"
)

type GitLock struct {
	StateFileId 	string
	LockBranch	string
	RemoteName 	string
}

func (gitLock GitLock) AcquireLock() error {
	tmpGitDir, err := createTmpDirForGitProject()
	if err != nil {
		return err
	}

	if err := cloneRepoToDir(tmpGitDir); err != nil {
		return err
	}

	if err := checkoutLockBranch(gitLock.LockBranch, tmpGitDir); err != nil {
		return err
	}

	for {
		err := tryToAcquireLock(gitLock, tmpGitDir)
		if err == nil {
			util.Logger.Printf("Lock acquired!")
			return nil
		} else if err.Error() == "foo" {
			// TODO: determine if error means someone else has the lock
			// TODO: undo last commit and delete lock file
			// TODO: display lock metadata by pulling down the existing file and showing its contents
			// TODO: sleep for 30 seconds and try again
			return err
		} else {
			return err
		}
	}
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

// Try to acquire a lock by creating a lock file and pushing it to Git. Returns the path to the created lock file or an
// empty string if an error occurred creating the file. Returns an error if the Git push failed.
func tryToAcquireLock(gitLock GitLock, gitDir string) error {
	if err := pullLatestFromGit(gitLock.LockBranch, gitLock.RemoteName, gitDir); err != nil {
		return err
	}

	if util.PathExists(lockFilePath(gitLock.StateFileId, gitDir)) {
		// TODO: handle case where lock file already exists
	}

	lockFile, err := createLockFile(gitLock.StateFileId, gitDir)
	if err != nil {
		return err
	}

	commitMessage := fmt.Sprintf("[terragrunt] Acquiring lock for state file %s", gitLock.StateFileId)
	return commitAndPushFileToGit(lockFile, gitDir, gitLock.RemoteName, gitLock.LockBranch, commitMessage)
}

// Commit and push the given file in the given directory to the given branch and remote name of Git.
func commitAndPushFileToGit(filePath string, gitDir string, remoteName string, branchName string, commitMessage string) error {
	util.Logger.Printf("Attempting to commit and push file %s to Git", filePath)

	if err := shell.RunShellCommand("git", "add", filePath, "--git-dir", gitDir); err != nil {
		return err
	}

	if err := shell.RunShellCommand("git", "commit", "-m", commitMessage, "--git-dir", gitDir); err != nil {
		return err
	}

	return shell.RunShellCommand("git", "push", remoteName, branchName, "--git-dir", gitDir)
}

func getLockFileMetadata(stateFileId string, gitDir string) (*locks.LockMetadata, error) {
	lockFilePath := lockFilePath(stateFileId, gitDir)
	bytes, err := ioutil.ReadFile(lockFilePath)
	if err != nil {
		return nil, err
	}

	lockFileMetadata := &locks.LockMetadata{}
	if err := json.Unmarshal(bytes, lockFileMetadata); err != nil {
		return nil, err
	}

	return lockFileMetadata, nil
}

// Create a lock file for the given stateFileId in gitDir. The file will contain metadata about the lock, such as who
// created it and when. Return the path to the lock file.
func createLockFile(stateFileId string, gitDir string) (string, error) {
	lockFilePath := lockFilePath(stateFileId, gitDir)
	util.Logger.Printf("Creating lock file at %s", lockFilePath)

	lockMetadata, err := locks.CreateLockMetadata(stateFileId)
	if err != nil {
		return lockFilePath, err
	}

	lockFileBytes, err := json.Marshal(lockMetadata)
	if err != nil {
		return lockFilePath, err
	}

	return lockFilePath, ioutil.WriteFile(lockFilePath, lockFileBytes, 0644)
}

// Delete the lock file for the given state file in the given git directory
func deleteLockFile(stateFileId string, gitDir string) error {
	lockFilePath := lockFilePath(stateFileId, gitDir)
	util.Logger.Printf("Deleting lock file at %s", lockFilePath)
	return os.Remove(lockFilePath)
}

// Create the path for a lock file for the given state file in the given git directory
func lockFilePath(stateFileId string, gitDir string) string {
	return path.Join(gitDir, fmt.Sprintf("%s-lock.json", stateFileId))
}

// Git pull the latest for the given branch from the given remote
func pullLatestFromGit(lockBranch string, remoteName string, gitDir string) error {
	util.Logger.Printf("Pulling latest code for lock branch %s in git folder %s", lockBranch, gitDir)
	return shell.RunShellCommand("git", "pull", remoteName, lockBranch, "--git-dir", gitDir)
}

// Checkout, or create if it doesn't exist, the given lock branch in the given git directory
func checkoutLockBranch(lockBranch string, gitDir string) error {
	util.Logger.Printf("Checking out lock branch %s in git folder %s", lockBranch, gitDir)
	// The -B flag will create the branch if it doesn't already exist: http://stackoverflow.com/a/26961416/483528
	return shell.RunShellCommand("git", "checkout", "-B", lockBranch, "--git-dir", gitDir)
}

// Clone the Git repo in the current directory to the given directory, which is assumed to be a temp directory we can
// use for scratch work. We make all our changes in a temp directory to ensure we don't mess up the user's local
// checkout.
func cloneRepoToDir(destDir string) error {
	isDirEmpty, err := util.IsDirEmpty(destDir)
	if err != nil {
		return err
	}

	if isDirEmpty {
		srcDir, err := getGitRootDir()
		if err != nil {
			return err
		}

		util.Logger.Printf("Cloning repo %s into %s.", srcDir, destDir)
		return shell.RunShellCommand("git", "clone", srcDir, destDir)
	} else {
		util.Logger.Printf("Folder %s alrady has files in it. Will not do a git clone.", destDir)
		return nil
	}
}

// Return the root directory of this git project. This is useful as the Terraform templates are often in a subfolder.
func getGitRootDir() (string, error) {
	output, err := shell.RunShellCommandAndGetOutput("git", "rev-parse", "--show-toplevel")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(output), nil
}

// Create the temp folder to use for the Git project in the current working directory and return the path to the temp
// folder. We do all of our changes in a temp folder to ensure we don't mess up the user's local checkout. We use this
// function to deterministically determine the name for the temp folder given a source directory so we can reuse that
// folder across runs.
func createTmpDirForGitProject() (string, error) {
	gitRootDir, err := getGitRootDir()
	if err != nil {
		return "", err
	}

	baseName := path.Base(gitRootDir)
	tmpDir := os.TempDir()

	tmpDirForRepo := path.Join(tmpDir, "terragrunt", baseName)
	return tmpDirForRepo, os.MkdirAll(tmpDirForRepo, 0777)
}