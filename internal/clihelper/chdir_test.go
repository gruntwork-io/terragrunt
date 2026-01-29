package clihelper_test

import (
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/clihelper"
	"github.com/stretchr/testify/assert"
)

func TestChdirConsumedAsFlagValue(t *testing.T) {
	t.Parallel()

	// Case 1: -chdir with space-separated value
	args1 := []string{"-chdir", "/tmp/dir", "apply"}
	iacArgs1 := clihelper.NewIacArgs(args1...)

	assert.Equal(t, "apply", iacArgs1.Command)
	assert.Contains(t, iacArgs1.Flags, "-chdir")
	assert.Contains(t, iacArgs1.Flags, "/tmp/dir")
	assert.NotContains(t, iacArgs1.Arguments, "/tmp/dir")

	// Case 2: -chdir with equals value
	args2 := []string{"-chdir=/tmp/dir", "plan"}
	iacArgs2 := clihelper.NewIacArgs(args2...)

	assert.Equal(t, "plan", iacArgs2.Command)
	assert.Contains(t, iacArgs2.Flags, "-chdir=/tmp/dir")
	assert.NotContains(t, iacArgs2.Arguments, "/tmp/dir")
}
