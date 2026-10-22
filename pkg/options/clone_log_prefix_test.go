package options_test

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/pkg/log/format/placeholders"
	"github.com/gruntwork-io/terragrunt/pkg/options"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCloneWithConfigPathLogPrefix pins that a clone keeps the logger's prefix
// while the config path stays the same, even after WorkingDir has moved to the
// cache dir, and replaces it when the config path names another unit.
func TestCloneWithConfigPathLogPrefix(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	unitDir := filepath.Join(root, "live", "basic", "hello")
	otherDir := filepath.Join(root, "live", "basic", "other")

	tc := []struct {
		name       string
		cloneTo    string
		wantPrefix string
	}{
		{
			name:       "same config keeps unit prefix",
			cloneTo:    filepath.Join(unitDir, "terragrunt.hcl"),
			wantPrefix: "live/basic/hello",
		},
		{
			name:       "other config takes its dir",
			cloneTo:    filepath.Join(otherDir, "terragrunt.hcl"),
			wantPrefix: otherDir,
		},
	}

	for _, tt := range tc {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			opts, err := options.NewTerragruntOptionsForTest(filepath.Join(unitDir, "terragrunt.hcl"))
			require.NoError(t, err)

			opts.WorkingDir = filepath.Join(unitDir, ".terragrunt-cache", "src")

			buf := new(bytes.Buffer)
			l := prefixLogger(t, buf).WithField(placeholders.WorkDirKeyName, "live/basic/hello")

			l, _, err = opts.CloneWithConfigPath(l, tt.cloneTo)
			require.NoError(t, err)

			l.Infof("probe")
			assert.Equal(t, tt.wantPrefix, strings.TrimSpace(buf.String()))
		})
	}
}

func prefixLogger(t *testing.T, buf *bytes.Buffer) log.Logger {
	t.Helper()

	l := logger.CreateLogger()
	require.NoError(t, l.Formatter().SetCustomFormat("%prefix"))
	l.SetOptions(log.WithOutput(buf))

	return l
}
