//go:build windows

package getter_test

import (
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/getter"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestDetectReturnsDrivePathUnchanged pins that a drive path passes through
// Detect as written. go-getter already parses it as a file URL, so it never
// reaches the file detector, and ToSourceURL re-parses it into a local source.
func TestDetectReturnsDrivePathUnchanged(t *testing.T) {
	t.Parallel()

	src := filepath.ToSlash(venvtest.Root("/abs/path/to/module"))

	got, err := getter.Detect(src, "/tmp")
	require.NoError(t, err)
	assert.Equal(t, src, got)
}
