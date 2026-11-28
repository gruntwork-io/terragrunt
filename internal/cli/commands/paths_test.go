package commands_test

import (
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"
)

// testWorkDir is the working dir the wrapper tests run in, absolute on the host OS.
var testWorkDir = venvtest.Root("/work")
