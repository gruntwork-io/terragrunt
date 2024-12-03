// Tests specific to race conditions are verified here

package test_test

import (
	"testing"

	"github.com/gruntwork-io/terragrunt/options"
	"github.com/stretchr/testify/require"
)

func TestStrictModeWithRacing(t *testing.T) {
	t.Parallel()

	opts, err := options.NewTerragruntOptionsForTest("race_test")
	require.NoError(t, err)

	go opts.AppendReadFile("file.json", "unit")
	go opts.AppendReadFile("file.json", "other-unit")
}
