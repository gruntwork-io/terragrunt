package helpers_test

import (
	"testing"

	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRunContainerSkipsWhenDisabled(t *testing.T) {
	var (
		skippedTest *testing.T
		reached     bool
	)

	t.Run("SKIP_TESTCONTAINERS set", func(t *testing.T) {
		skippedTest = t

		t.Setenv("SKIP_TESTCONTAINERS", "1")

		// The skip comes before any Docker call, so no daemon is needed.
		helpers.RunContainer(t, "thT-image-never-pulled:latest", 8080)

		reached = true
	})

	require.NotNil(t, skippedTest)
	assert.True(t, skippedTest.Skipped())
	assert.False(t, reached, "RunContainer must stop the test before starting a container")
}
