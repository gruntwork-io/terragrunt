package commands_test

import (
	"bytes"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/cli/commands"
	"github.com/gruntwork-io/terragrunt/internal/tf"
	"github.com/gruntwork-io/terragrunt/internal/tfimpl"
	"github.com/gruntwork-io/terragrunt/internal/tips"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/pkg/log/format"
	"github.com/gruntwork-io/terragrunt/pkg/log/format/placeholders"
	"github.com/hashicorp/go-version"
	"github.com/stretchr/testify/assert"
)

func TestGiveWindowsSymlinksTip(t *testing.T) {
	t.Parallel()

	cacheEnv := map[string]string{tf.EnvNameTFPluginCacheDir: "/some/cache/dir"}
	emptyEnv := map[string]string{}

	openTofuV1120 := version.Must(version.NewVersion("1.12.0"))
	openTofuV1110 := version.Must(version.NewVersion("1.11.0"))

	defaultMessage := "Windows users may encounter silent fallback behavior to provider copying instead of symlinking in " +
		"OpenTofu/Terraform. See https://github.com/gruntwork-io/terragrunt/issues/5061 for more information."
	openTofuMessage := "Windows users may encounter silent fallback from symlinking to copying for provider plugins. " +
		"Set TF_LOG=warn to check if OpenTofu is falling back to copying. " +
		"See https://github.com/gruntwork-io/terragrunt/issues/5061 for more information."

	testCases := []struct {
		fs                   vfs.FS
		environ              map[string]string
		tfVersion            *version.Version
		name                 string
		goos                 string
		tfImpl               tfimpl.Type
		expectedMessage      string
		providerCacheEnabled bool
	}{
		{
			name:                 "non-windows skips",
			goos:                 "linux",
			fs:                   vfs.NewMemMapFS(),
			environ:              cacheEnv,
			providerCacheEnabled: true,
			tfImpl:               tfimpl.Terraform,
			expectedMessage:      "",
		},
		{
			name:                 "windows with no cache skips",
			goos:                 "windows",
			fs:                   vfs.NewMemMapFS(),
			environ:              emptyEnv,
			providerCacheEnabled: false,
			tfImpl:               tfimpl.Terraform,
			expectedMessage:      "",
		},
		{
			name:                 "windows with cache and symlink works skips",
			goos:                 "windows",
			fs:                   vfs.NewMemMapFS(),
			environ:              cacheEnv,
			providerCacheEnabled: true,
			tfImpl:               tfimpl.Terraform,
			expectedMessage:      "",
		},
		{
			name:                 "windows with cache and symlink fails shows default tip",
			goos:                 "windows",
			fs:                   &vfs.NoSymlinkFS{FS: vfs.NewMemMapFS()},
			environ:              cacheEnv,
			providerCacheEnabled: true,
			tfImpl:               tfimpl.Terraform,
			expectedMessage:      defaultMessage,
		},
		{
			name:                 "windows symlink fails with OpenTofu >= 1.12 shows OpenTofu tip",
			goos:                 "windows",
			fs:                   &vfs.NoSymlinkFS{FS: vfs.NewMemMapFS()},
			environ:              cacheEnv,
			providerCacheEnabled: true,
			tfImpl:               tfimpl.OpenTofu,
			tfVersion:            openTofuV1120,
			expectedMessage:      openTofuMessage,
		},
		{
			name:                 "windows symlink fails with OpenTofu < 1.12 shows default tip",
			goos:                 "windows",
			fs:                   &vfs.NoSymlinkFS{FS: vfs.NewMemMapFS()},
			environ:              cacheEnv,
			providerCacheEnabled: true,
			tfImpl:               tfimpl.OpenTofu,
			tfVersion:            openTofuV1110,
			expectedMessage:      defaultMessage,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			logger, output := newTestLogger()
			allTips := tips.NewTips()

			commands.GiveWindowsSymlinksTip(
				logger,
				tc.fs,
				tc.goos,
				allTips,
				tc.environ,
				tc.providerCacheEnabled,
				tc.tfImpl,
				tc.tfVersion,
			)

			content := output.String()

			if tc.expectedMessage == "" {
				assert.Empty(t, content)
				return
			}

			assert.Contains(t, content, tc.expectedMessage,
				"expected output to contain %q, got %q", tc.expectedMessage, content)
		})
	}
}

func newTestLogger() (log.Logger, *bytes.Buffer) {
	formatter := format.NewFormatter(placeholders.Placeholders{placeholders.Message()})
	output := new(bytes.Buffer)
	logger := log.New(log.WithOutput(output), log.WithLevel(log.InfoLevel), log.WithFormatter(formatter))

	return logger, output
}
