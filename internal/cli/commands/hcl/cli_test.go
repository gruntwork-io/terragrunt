package hcl_test

import (
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/cli/commands/hcl"
	"github.com/gruntwork-io/terragrunt/internal/vexec"
	"github.com/gruntwork-io/terragrunt/pkg/options"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestHclCommandSkipsAutoProviderCacheDir pins that the `hcl` command disables
// the auto provider cache dir before RunAction, so `hcl format` and
// `hcl validate` never probe `tofu -version` at startup (issue #7094).
func TestHclCommandSkipsAutoProviderCacheDir(t *testing.T) {
	t.Parallel()

	opts := options.NewTerragruntOptions(vexec.NewOSExec())

	cmd := hcl.NewCommand(logger.CreateLogger(), opts, venvtest.New())

	require.NotNil(t, cmd.Before, "hcl command must opt out of the auto provider cache dir in Before")
	require.NoError(t, cmd.Before(t.Context(), nil))
	assert.True(t, opts.NoAutoProviderCacheDir)
}
