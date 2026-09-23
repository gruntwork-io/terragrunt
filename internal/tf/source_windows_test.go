//go:build windows

package tf_test

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gruntwork-io/terragrunt/internal/tf"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"
)

// TestToSourceURLKeepsDriveOfLocalPath pins that a forward-slash drive path
// becomes a local source that still names its drive.
func TestToSourceURLKeepsDriveOfLocalPath(t *testing.T) {
	t.Parallel()

	modulePath := filepath.ToSlash(venvtest.Root("/abs/path/to/module"))

	sourceURL, err := tf.ToSourceURL(modulePath, venvtest.Root("/work"))
	require.NoError(t, err)

	assert.True(t, tf.IsLocalSource(sourceURL))
	assert.Equal(t, modulePath, sourceURL.Path)
}
