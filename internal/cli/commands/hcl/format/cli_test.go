package format_test

import (
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/cli/commands/hcl/format"
	"github.com/gruntwork-io/terragrunt/internal/vexec"
	"github.com/gruntwork-io/terragrunt/pkg/options"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNewCommandSkipsAutoProviderCacheDir pins that `hcl format` opts out of
// the auto provider cache dir, so it never probes a binary at startup (#7094).
func TestNewCommandSkipsAutoProviderCacheDir(t *testing.T) {
	t.Parallel()

	opts := options.NewTerragruntOptions(vexec.NewOSExec())

	cmd := format.NewCommand(logger.CreateLogger(), opts, venvtest.New())

	require.NotNil(t, cmd.Before, "format command must opt out of the auto provider cache dir in Before")
	require.NoError(t, cmd.Before(t.Context(), nil))
	assert.True(t, opts.NoAutoProviderCacheDir)
}
