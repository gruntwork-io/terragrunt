package util

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestExistingCommand(t *testing.T) {
	t.Parallel()

	assert.True(t, IsCommandExecutable("pwd"))
}

func TestNotExistingCommand(t *testing.T) {
	t.Parallel()

	assert.False(t, IsCommandExecutable("not-existing-command", "--version"))
}
