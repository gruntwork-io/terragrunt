package cas

import (
	"github.com/gruntwork-io/terragrunt/internal/git"
	"github.com/gruntwork-io/terragrunt/internal/vexec"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
)

// Venv bundles the virtualized dependencies CAS operations need so callers
// pass both per call rather than CAS holding them as struct fields. Either
// field can be a stub in tests.
type Venv struct {
	// FS is the filesystem CAS reads and writes through.
	FS vfs.FS
	// Git shells out to the git binary.
	Git *git.GitRunner
}

// OSVenv builds the production [Venv]: the real OS filesystem and a git
// runner backed by [vexec.NewOSExec]. Returns an error if the git binary
// is not on PATH.
func OSVenv() (Venv, error) {
	runner, err := git.NewGitRunner(vexec.NewOSExec())
	if err != nil {
		return Venv{}, err
	}

	return Venv{FS: vfs.NewOSFS(), Git: runner}, nil
}
