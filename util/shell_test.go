package util_test

import (
	"testing"

	"github.com/gruntwork-io/terragrunt/util"
	"github.com/stretchr/testify/assert"
)

func TestExistingCommand(t *testing.T) {
	t.Parallel()

	assert.True(t, util.IsCommandExecutable(t.Context(), "pwd"))
}

func TestNotExistingCommand(t *testing.T) {
	t.Parallel()

	assert.False(t, util.IsCommandExecutable(t.Context(), "not-existing-command", "--version"))
}
