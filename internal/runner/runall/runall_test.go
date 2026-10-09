package runall_test

import (
	"bytes"
	"errors"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/filter"
	"github.com/gruntwork-io/terragrunt/internal/runner/runall"
	"github.com/gruntwork-io/terragrunt/internal/tips"
	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/pkg/log/format"
	"github.com/gruntwork-io/terragrunt/pkg/log/format/placeholders"
	"github.com/gruntwork-io/terragrunt/pkg/options"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMissingRunAllArguments(t *testing.T) {
	t.Parallel()

	tgOptions, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	tgOptions.TerraformCommand = ""

	err = runall.Run(t.Context(), logger.CreateLogger(), venvtest.New(), tgOptions)
	require.Error(t, err)

	var missingCommand runall.MissingCommand

	ok := errors.As(err, &missingCommand)
	fmt.Println(err, errors.Unwrap(err))
	assert.True(t, ok)
}

// TestRunGivesStackTargetTipOnEarlyReturn pins that `run --all` still emits the
// stack target tip when it returns before running anything.
func TestRunGivesStackTargetTipOnEarlyReturn(t *testing.T) {
	t.Parallel()

	root := venvtest.Root("/repo")
	v := venvtest.New().WithFS(venvtest.NewFS(t, root, map[string]string{
		filepath.Join("envs", "prod", config.DefaultStackFile): "",
	}))

	opts, err := options.NewTerragruntOptionsForTest(filepath.Join(root, config.DefaultTerragruntConfigPath))
	require.NoError(t, err)

	stackFilter, err := filter.Parse("./envs/prod")
	require.NoError(t, err)

	opts.WorkingDir = root
	opts.TerraformCommand = ""
	opts.Filters = filter.Filters{stackFilter}

	output := new(bytes.Buffer)
	l := log.New(
		log.WithOutput(output),
		log.WithLevel(log.InfoLevel),
		log.WithFormatter(format.NewFormatter(placeholders.Placeholders{placeholders.Message()})),
	)

	err = runall.Run(t.Context(), l, v, opts)

	_, missingCommand := errors.AsType[runall.MissingCommand](err)
	require.True(t, missingCommand, "unexpected error %v", err)
	assert.Contains(t, output.String(), "TIP ("+tips.StackTargetMissingTypeStack+")")
}
