//go:build !windows

package getter_test

import (
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/getter"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestDetectReattachesFileSchemeToAbsolutePath pins the fileSchemeDetector
// wrapper in defaultDetectors(): v2 dropped v1's "file://" scheme on raw
// FileDetector output, and IsLocalSource needs it to recognize a local source.
func TestDetectReattachesFileSchemeToAbsolutePath(t *testing.T) {
	t.Parallel()

	got, err := getter.Detect("/abs/path/to/module", "/tmp")
	require.NoError(t, err)
	assert.Equal(t, "file:///abs/path/to/module", got)
}
