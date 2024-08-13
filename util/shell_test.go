package util_test

import (
	"testing"

	"github.com/gruntwork-io/terragrunt/util"
	"github.com/stretchr/testify/assert"
)

func TestExistingCommand(t *testing.T) {
	t.Parallel()

	assert.True(t, util.IsCommandExecutable("pwd"))
}

func TestNotExistingCommand(t *testing.T) {
	t.Parallel()

	assert.False(t, util.IsCommandExecutable("not-existing-command", "--version"))
}
