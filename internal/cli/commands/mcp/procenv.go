package mcp

import (
	"errors"
	"fmt"
	"maps"
	"path/filepath"
	"slices"
	"strings"

	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
)

// ownerOnlyDirPerms keeps the empty home and PATH directories writable only by
// the invoking user.
const ownerOnlyDirPerms = 0o700

// homeVars are the variables the Go standard library and the cloud SDKs
// resolve the invoking user's home and config directories from.
var homeVars = []string{"HOME", "USERPROFILE", "APPDATA", "LOCALAPPDATA"}

// keptVars survive [IsolatedEnv] without [CapabilityEnv]. They name the
// temporary directory, which os.TempDir on Windows replaces with the Windows
// directory when they are missing, and SystemRoot, which Windows programs need
// to load system libraries.
var keptVars = []string{"TMPDIR", "TMP", "TEMP", "SystemRoot"}

// IsolatedEnv returns the process environment the server runs under, given
// its own environment env, the capabilities granted, and two empty
// directories. It covers the libraries that read the process environment
// rather than the venv: go-getter runs git with it, and the AWS, GCP, and
// Azure SDKs find credentials in it and in the home directory.
//
// Without [CapabilityEnv], only the variables naming the temporary directory
// and SystemRoot survive, plus PATH when [CapabilityExec] is granted, and every
// home directory variable points at home. Without [CapabilityExec], PATH points
// at bin, so a library spawning a bare program name finds nothing.
//
// Names match case-insensitively, as Windows matches them.
func IsolatedEnv(env map[string]string, granted Capabilities, home, bin string) map[string]string {
	isolated := maps.Clone(env)

	if !granted.Has(CapabilityEnv) {
		kept := keptVars
		if granted.Has(CapabilityExec) {
			kept = append([]string{"PATH"}, keptVars...)
		}

		maps.DeleteFunc(isolated, func(name, _ string) bool {
			return !slices.ContainsFunc(kept, func(k string) bool { return strings.EqualFold(k, name) })
		})

		for _, name := range homeVars {
			setFold(isolated, name, home)
		}
	}

	if !granted.Has(CapabilityExec) {
		setFold(isolated, "PATH", bin)
	}

	return isolated
}

// isolateProcessEnv replaces the process environment with the one
// [IsolatedEnv] returns for granted, backed by fresh empty directories. The
// returned func removes those directories.
//
// It changes the environment of the whole process, so it runs once, before
// the server starts.
func isolateProcessEnv(v *venv.Venv, granted Capabilities) (func() error, error) {
	v.RequireFS()
	v.RequireTempDir()
	v.RequireReplaceEnviron()

	if granted.Has(CapabilityEnv) && granted.Has(CapabilityExec) {
		return func() error { return nil }, nil
	}

	root, err := vfs.MkdirTemp(v.FS, v.Platform.TempDir(), "terragrunt-mcp-env-")
	if err != nil {
		return nil, fmt.Errorf("creating the server's empty home and PATH directories: %w", err)
	}

	cleanup := func() error { return v.FS.RemoveAll(root) }

	home := filepath.Join(root, "home")
	bin := filepath.Join(root, "bin")

	for _, dir := range []string{home, bin} {
		if err := v.FS.MkdirAll(dir, ownerOnlyDirPerms); err != nil {
			return nil, errors.Join(
				fmt.Errorf("creating the server's empty home and PATH directories: %w", err),
				cleanup(),
			)
		}
	}

	if err := v.Platform.ReplaceEnviron(IsolatedEnv(v.Env, granted, home, bin)); err != nil {
		return nil, errors.Join(err, cleanup())
	}

	return cleanup, nil
}

// setFold sets name in env after deleting every key that differs from it only
// in case. Windows reports PATH as Path, and setting both would leave
// whichever os.Setenv applies last.
func setFold(env map[string]string, name, value string) {
	maps.DeleteFunc(env, func(key, _ string) bool { return strings.EqualFold(key, name) })
	env[name] = value
}
