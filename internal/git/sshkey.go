package git

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"maps"
	"path/filepath"

	"github.com/gruntwork-io/terragrunt/internal/semver"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
)

const (
	// EnvGitSSHCommand is the variable git reads its ssh invocation from.
	EnvGitSSHCommand = "GIT_SSH_COMMAND"

	// ErrInvalidSSHKey is returned when an sshkey query value is not base64.
	ErrInvalidSSHKey Error = "sshkey is not base64"
	// ErrGitTooOldForSSHKey is returned when a source sets the sshkey query
	// parameter and git is older than [MinGitVersionForSSHKey].
	ErrGitTooOldForSSHKey Error = "using an ssh key needs a newer git"

	// sshKeyPerms keeps a decoded sshkey readable by its owner only.
	sshKeyPerms = 0o600
)

// MinGitVersionForSSHKey is the first git version that reads GIT_SSH_COMMAND,
// which is how an sshkey reaches ssh.
var MinGitVersionForSSHKey = semver.MustParse("2.3")

// RequireSSHKeySupport fails unless git is at least [MinGitVersionForSSHKey].
//
// Returns [ErrGitTooOldForSSHKey] when it is older.
func (g *GitRunner) RequireSSHKeySupport(ctx context.Context) error {
	have, err := g.Version(ctx)
	if err != nil {
		return err
	}

	if have.LessThan(MinGitVersionForSSHKey) {
		return fmt.Errorf("%w: need git %s, have %s", ErrGitTooOldForSSHKey, MinGitVersionForSSHKey, have)
	}

	return nil
}

// WriteSSHKey decodes the base64 sshkey query value into a temporary file only
// its owner can read, and returns the file's path. The caller removes the file.
//
// Returns [ErrInvalidSSHKey] when encoded is not base64.
func WriteSSHKey(v *venv.Venv, encoded string) (string, error) {
	raw, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return "", ErrInvalidSSHKey
	}

	f, err := vfs.CreateTemp(v.FS, v.Platform.TempDir(), "terragrunt-git-sshkey-")
	if err != nil {
		return "", err
	}

	path := f.Name()

	if err := v.FS.Chmod(path, sshKeyPerms); err != nil {
		return "", errors.Join(err, f.Close(), v.FS.Remove(path))
	}

	if _, err := f.Write(raw); err != nil {
		return "", errors.Join(err, f.Close(), v.FS.Remove(path))
	}

	if err := f.Close(); err != nil {
		return "", errors.Join(err, v.FS.Remove(path))
	}

	return path, nil
}

// SSHEnv returns a copy of env with GIT_SSH_COMMAND set to the ssh command
// already in env, or plain ssh, followed by `-i keyFile` when keyFile is not
// empty.
//
// The variable is set even without a key, so it overrides a core.sshCommand in
// git config.
func SSHEnv(env map[string]string, keyFile string) map[string]string {
	sshCmd := env[EnvGitSSHCommand]
	if sshCmd == "" {
		sshCmd = "ssh"
	}

	if keyFile != "" {
		sshCmd += " -i " + filepath.ToSlash(keyFile)
	}

	out := maps.Clone(env)
	out[EnvGitSSHCommand] = sshCmd

	return out
}
