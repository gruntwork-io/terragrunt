package util_test

import (
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/util"
	"github.com/gruntwork-io/terragrunt/internal/vexec"
	"github.com/stretchr/testify/assert"
)

func TestExistingCommand(t *testing.T) {
	t.Parallel()

	assert.True(t, util.IsCommandExecutable(vexec.NewOSExec(), t.Context(), "pwd"))
}

func TestNotExistingCommand(t *testing.T) {
	t.Parallel()

	assert.False(t, util.IsCommandExecutable(vexec.NewOSExec(), t.Context(), "not-existing-command", "--version"))
}
