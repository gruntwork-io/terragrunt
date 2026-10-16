package tips

import (
	"path/filepath"

	semver "github.com/gruntwork-io/terragrunt/internal/semver"
	"github.com/gruntwork-io/terragrunt/internal/tf"
	"github.com/gruntwork-io/terragrunt/internal/tfimpl"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/pkg/log"
)

// probeDirPerms is the mode of the throwaway directory the symlink probe creates.
const probeDirPerms = 0o755

// GiveWindowsSymlinksTip warns Windows users that OpenTofu/Terraform may not create symlinks
// for provider plugins installed in the local cache directory.
//
// We generally don't need to recommend this if:
//   - The user is not on Windows (this is generally a Windows-only problem)
//   - The user isn't using the provider cache directory or provider cache server
//   - We can successfully create a test symlink
//
// Note that OpenTofu doesn't want to emit a warning like this for OpenTofu users.
//
// See: https://github.com/opentofu/opentofu/issues/3972
//
// In the future, this may be less important, as the OpenTofu global provider cache
// dir might be the default, and no copying/symlinking will happen anyways. At that
// time, this check may need to have a version gate to avoid warning Windows users
// on a sufficiently new version of OpenTofu.
func GiveWindowsSymlinksTip(
	l log.Logger,
	fsys vfs.FS,
	goos string,
	allTips Tips,
	envs map[string]string,
	providerCacheEnabled bool,
	tfImpl tfimpl.Type,
	tfVersion *semver.Version,
) {
	if goos != "windows" {
		return
	}

	if envs[tf.EnvNameTFPluginCacheDir] == "" && !providerCacheEnabled {
		return
	}

	tmp, err := vfs.MkdirTemp(fsys, "", "terragrunt-test-symlink")
	if err != nil {
		l.Debugf("Failed to create temporary directory for testing symlink: %v", err)
		return
	}

	defer func() {
		if err := fsys.RemoveAll(tmp); err != nil {
			l.Debugf("Failed to remove temporary directory for testing symlink: %v", err)
		}
	}()

	source := filepath.Join(tmp, "source")
	target := filepath.Join(tmp, "target")

	if err := fsys.Mkdir(source, probeDirPerms); err != nil {
		l.Debugf("Failed to create source directory for testing symlink: %v", err)
		return
	}

	err = vfs.Symlink(fsys, source, target)
	if err == nil {
		return
	}

	tip := allTips.Find(WindowsSymlinkWarning)
	if tip == nil {
		return
	}

	// EvaluateWith keeps the shared Tip.Message untouched, since concurrent unit runs reach this path.
	msg := tip.Message

	if tfImpl == tfimpl.OpenTofu && tfVersion != nil {
		minVersion, verErr := semver.Parse("1.12.0")
		if verErr == nil && !tfVersion.LessThan(minVersion) {
			msg = WindowsSymlinkWarningOpenTofuMessage
		}
	}

	tip.EvaluateWith(l, msg)
}
