package cas

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io/fs"

	"github.com/gruntwork-io/terragrunt/internal/git"
	"github.com/gruntwork-io/terragrunt/internal/venv"
)

// withSSHKey calls fn with a copy of v whose git connects with the base64 key
// encoded, or with v itself when encoded is empty.
//
// A key is on disk only while fn runs. Run this inside a flight rather than
// around it: a flight outlives a caller that gives up, so a key file the
// caller owned would vanish from under the callers still waiting.
func withSSHKey[T any](
	ctx context.Context,
	v *venv.Venv,
	encoded string,
	fn func(*venv.Venv) (T, error),
) (val T, err error) {
	if encoded == "" {
		return fn(v)
	}

	v.RequireEnv()
	v.RequireTempDir()

	runner, err := git.NewGitRunner(v)
	if err != nil {
		return val, err
	}

	if err := runner.RequireSSHKeySupport(ctx); err != nil {
		return val, err
	}

	keyFile, err := git.WriteSSHKey(v, encoded)
	if err != nil {
		return val, err
	}

	defer func() {
		if rmErr := v.FS.Remove(keyFile); rmErr != nil && !errors.Is(rmErr, fs.ErrNotExist) {
			err = errors.Join(err, rmErr)
		}
	}()

	return fn(v.WithEnv(git.SSHEnv(v.Env, keyFile)))
}

// sshKeyFlightScope returns the part of a flight key that separates callers by
// the key they connect with, so nobody waits on a flight that authenticates as
// someone else. The key is hashed to keep it out of the flight map.
func sshKeyFlightScope(encoded string) string {
	if encoded == "" {
		return ""
	}

	sum := sha256.Sum256([]byte(encoded))

	return "\x00sshkey\x00" + hex.EncodeToString(sum[:])
}
