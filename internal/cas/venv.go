package cas

import (
	"errors"

	"github.com/gruntwork-io/terragrunt/internal/git"
	"github.com/gruntwork-io/terragrunt/internal/vexec"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
)

// ErrVenvFSUnset is the panic value [Venv.RequireFS] raises when a
// function declares it needs v.FS and the caller hands in a Venv with
// FS == nil. Production callers build Venv through [OSVenv], so the
// panic surfaces a test misconfiguration rather than a runtime
// condition.
var ErrVenvFSUnset = errors.New("cas.Venv.FS is required but unset")

// ErrVenvGitUnset is the panic value [Venv.RequireGit] raises when a
// function declares it needs v.Git and the caller hands in a Venv with
// Git == nil. Production callers build Venv through [OSVenv], so the
// panic surfaces a test misconfiguration rather than a runtime
// condition.
var ErrVenvGitUnset = errors.New("cas.Venv.Git is required but unset")

// Venv bundles the virtualized dependencies CAS operations need so callers
// pass both per call rather than CAS holding them as struct fields. Either
// field can be a stub in tests.
//
// Functions document which handles they touch and call [Venv.RequireFS]
// or [Venv.RequireGit] at entry so a missing handle panics at the
// offending call site instead of inside an unrelated stack frame.
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

// RequireFS panics with [ErrVenvFSUnset] when v.FS is nil. Functions
// that touch the filesystem call this as their first statement so the
// contract sits next to the signature.
func (v Venv) RequireFS() {
	if v.FS == nil {
		panic(ErrVenvFSUnset)
	}
}

// RequireGit panics with [ErrVenvGitUnset] when v.Git is nil.
// Functions that shell out to git call this as their first statement so
// the contract sits next to the signature.
func (v Venv) RequireGit() {
	if v.Git == nil {
		panic(ErrVenvGitUnset)
	}
}
