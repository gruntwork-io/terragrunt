package runner_test

import (
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/runner"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUnitEarlyExitError_WithDependency(t *testing.T) {
	t.Parallel()

	err := runner.UnitEarlyExitError{
		UnitPath:         "/units/app",
		FailedDependency: "/units/vpc",
	}

	msg := err.Error()
	assert.Contains(t, msg, "/units/app")
	assert.Contains(t, msg, "/units/vpc")
}

func TestUnitEarlyExitError_WithoutDependency(t *testing.T) {
	t.Parallel()

	err := runner.UnitEarlyExitError{
		UnitPath: "/units/app",
	}

	msg := err.Error()
	assert.Contains(t, msg, "/units/app")
	assert.Contains(t, msg, "earlier failure")
}

func TestUnitFailedError(t *testing.T) {
	t.Parallel()

	err := runner.UnitFailedError{
		UnitPath: "/units/vpc",
	}

	msg := err.Error()
	assert.Contains(t, msg, "/units/vpc")
	assert.Contains(t, msg, "error during its run")
}

func TestNewUnitFailedError(t *testing.T) {
	t.Parallel()

	err := runner.NewUnitFailedError("/units/db")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "/units/db")
}

func TestNewUnitEarlyExitError(t *testing.T) {
	t.Parallel()

	err := runner.NewUnitEarlyExitError("/units/app", "/units/vpc")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "/units/app")
	assert.Contains(t, err.Error(), "/units/vpc")
}
