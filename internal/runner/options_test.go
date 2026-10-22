package runner_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gruntwork-io/terragrunt/internal/runner"
)

func TestWithGraphTarget(t *testing.T) {
	t.Parallel()

	opt := runner.WithGraphTarget("/repo/vpc")

	target, ok := opt.(interface{ GraphTarget() string })
	require.True(t, ok, "discovery reads the graph target through a GraphTarget accessor")
	assert.Equal(t, "/repo/vpc", target.GraphTarget())
}
