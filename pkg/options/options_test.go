package options_test

import (
	"testing"

	"github.com/gruntwork-io/terragrunt/pkg/options"
	"github.com/stretchr/testify/assert"
)

func TestInsertTerraformCliArgsSubcommandReplacement(t *testing.T) {
	t.Parallel()

	opts := options.NewTerragruntOptions()
	opts.TerraformCliArgs.SetCommand("providers")
	opts.TerraformCliArgs.AppendSubCommand("lock")

	// Original: providers lock
	assert.Equal(t, "providers", opts.TerraformCliArgs.Command)
	assert.Equal(t, []string{"lock"}, opts.TerraformCliArgs.SubCommand)

	// Insert "mirror". Should replace "lock".
	opts.InsertTerraformCliArgs("mirror")

	// Expected: providers mirror
	assert.Equal(t, "providers", opts.TerraformCliArgs.Command)
	assert.Equal(t, []string{"mirror"}, opts.TerraformCliArgs.SubCommand)

	// Ensure "lock" is gone or at least not appended (providers lock mirror).
}
