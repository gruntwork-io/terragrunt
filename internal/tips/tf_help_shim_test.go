package tips

import (
	"bytes"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/pkg/log/format"
	"github.com/gruntwork-io/terragrunt/pkg/log/format/placeholders"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGiveTFHelpShimTip(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name        string
		tfPath      string
		pathEnv     string
		contents    []byte
		disableTip  bool
		expectShown bool
	}{
		{
			name:        "text shim resolved from PATH",
			tfPath:      "tofu",
			pathEnv:     "/shims",
			contents:    []byte("#!/bin/sh\nexec tenv tofu \"$@\"\n"),
			expectShown: true,
		},
		{
			name:        "binary does not show tip",
			tfPath:      "/bin/tofu",
			contents:    []byte{'\x7f', 'E', 'L', 'F', 0x02, 0x01, 0x01, 0x00},
			expectShown: false,
		},
		{
			name:        "disabled tip is not shown",
			tfPath:      "./bin/tofu",
			contents:    []byte("#!/bin/sh\nexec tofu \"$@\"\n"),
			disableTip:  true,
			expectShown: false,
		},
		{
			name:        "missing binary does not show tip",
			tfPath:      "tofu",
			pathEnv:     "/shims",
			expectShown: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			fs := vfs.NewMemMapFS()
			path := tc.tfPath
			if tc.tfPath == "tofu" {
				path = "/shims/tofu"
			} else if tc.tfPath == "./bin/tofu" {
				path = "/work/bin/tofu"
			}

			if tc.contents != nil {
				require.NoError(t, fs.MkdirAll("/shims", 0o755))
				require.NoError(t, fs.MkdirAll("/work/bin", 0o755))
				require.NoError(t, vfs.WriteFile(fs, path, tc.contents, 0o755))
			}

			allTips := NewTips()
			if tc.disableTip {
				require.NoError(t, allTips.DisableTip(OpenTofuTerraformShim))
			}

			logger, output := newShimTestLogger()
			GiveTFHelpShimTip(logger, fs, tc.tfPath, "/work", tc.pathEnv, allTips)

			if tc.expectShown {
				assert.Contains(t, output.String(), OpenTofuTerraformShimMessage)
			} else {
				assert.Empty(t, output.String())
			}
		})
	}
}

func TestGiveTFHelpShimTipSkipsNonExecutablePathCandidate(t *testing.T) {
	t.Parallel()

	fs := vfs.NewMemMapFS()
	require.NoError(t, fs.MkdirAll("/shims", 0o755))
	require.NoError(t, fs.MkdirAll("/bin", 0o755))
	require.NoError(t, vfs.WriteFile(fs, "/shims/tofu", []byte("#!/bin/sh\nexec tenv tofu \"$@\"\n"), 0o644))
	require.NoError(t, vfs.WriteFile(fs, "/bin/tofu", []byte{'\x7f', 'E', 'L', 'F', 0x02, 0x01, 0x01, 0x00}, 0o755))

	logger, output := newShimTestLogger()
	GiveTFHelpShimTip(logger, fs, "tofu", "/work", "/shims:/bin", NewTips())

	assert.Empty(t, output.String())
}

func newShimTestLogger() (log.Logger, *bytes.Buffer) {
	formatter := format.NewFormatter(placeholders.Placeholders{placeholders.Message()})
	output := new(bytes.Buffer)
	logger := log.New(log.WithOutput(output), log.WithLevel(log.InfoLevel), log.WithFormatter(formatter))

	return logger, output
}
