//go:build tf

package config_test

import (
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/stretchr/testify/require"
)

// TestTFExposedIncludeFullParseSurfacesNoOutputsError pins that a full parse of a child
// config whose exposed include cannot resolve its dependency outputs returns a
// [config.TerragruntOutputTargetNoOutputs] error in the chain.
func TestTFExposedIncludeFullParseSurfacesNoOutputsError(t *testing.T) {
	t.Parallel()

	childPath, err := filepath.Abs(
		filepath.Join(
			"..",
			"..",
			"test",
			"fixtures",
			"regressions",
			"exposed-include-partial-parse-error",
			"child",
			"terragrunt.hcl",
		),
	)
	require.NoError(t, err)

	ctx, pctx := newTestParsingContext(t, venv.OSVenv(), childPath)
	pctx.TFPath = helpers.WrappedBinary(ctx)

	_, err = config.ParseConfigFile(ctx, pctx, logger.CreateLogger(), childPath, nil)
	require.Error(t, err)

	var noOutputs config.TerragruntOutputTargetNoOutputs
	require.ErrorAs(t, err, &noOutputs)
}
