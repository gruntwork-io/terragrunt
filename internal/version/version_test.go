package version_test

import (
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/version"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetVersion_ReturnsBuildVersion(t *testing.T) {
	t.Parallel()

	got := version.GetVersion()

	require.NotEmpty(t, got, "the version should never be empty")
	assert.Equal(t, version.Version, got, "GetVersion should report the ldflags-set Version")
}
