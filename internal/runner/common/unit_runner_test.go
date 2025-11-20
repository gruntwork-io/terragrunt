package common_test

import (
	"bytes"
	"context"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/errors"

	"github.com/stretchr/testify/require"

	"github.com/gruntwork-io/terragrunt/internal/component"
	"github.com/gruntwork-io/terragrunt/internal/report"
	"github.com/gruntwork-io/terragrunt/internal/runner/common"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"

	"github.com/stretchr/testify/assert"
)

// mockUnit is a minimal mock for Unit to test UnitRunner logic
// You may want to expand this for more complex tests
func newMockUnit() *component.Unit {
	unit := component.NewUnit("mock/path")
	unit.SetTerragruntOptions(&options.TerragruntOptions{})

	return unit
}

func TestNewUnitRunner(t *testing.T) {
	t.Parallel()

	unit := newMockUnit()
	runner := common.NewUnitRunner(unit)
	assert.Equal(t, unit, runner.Unit)
	assert.Equal(t, common.Waiting, runner.Status)
}

func TestUnitRunner_Run_AssumeAlreadyApplied(t *testing.T) {
	t.Parallel()

	unit := newMockUnit()
	unit.SetAssumeAlreadyApplied(true)
	runner := common.NewUnitRunner(unit)
	report := &report.Report{}
	err := runner.Run(t.Context(), logger.CreateLogger(), &options.TerragruntOptions{}, report)
	require.NoError(t, err)
	assert.Equal(t, common.Running, runner.Status)
}

func TestUnitRunner_Run_ErrorFromRunTerragrunt(t *testing.T) {
	t.Parallel()

	unit := newMockUnit()
	runner := common.NewUnitRunner(unit)
	path := t.TempDir()
	unit.SetPath(path)
	rep := report.NewReport().WithWorkingDir(path)
	err := runner.Run(t.Context(), logger.CreateLogger(), &options.TerragruntOptions{
		Writer: &bytes.Buffer{},
		RunTerragrunt: func(ctx context.Context, l log.Logger, opts *options.TerragruntOptions, r *report.Report) error {
			return errors.New("fail")
		},
	}, rep)
	require.Error(t, err)
	assert.Equal(t, common.Running, runner.Status)
	assert.Contains(t, err.Error(), "fail")
}

func TestUnitRunner_Run_Success(t *testing.T) {
	t.Parallel()

	unit := newMockUnit()
	path := t.TempDir()
	unit.SetPath(path)
	runner := common.NewUnitRunner(unit)
	rep := report.NewReport().WithWorkingDir(path)
	err := runner.Run(t.Context(), logger.CreateLogger(), &options.TerragruntOptions{
		Writer: &bytes.Buffer{},
		RunTerragrunt: func(ctx context.Context, l log.Logger, opts *options.TerragruntOptions, r *report.Report) error {
			return nil
		},
	}, rep)
	require.NoError(t, err)
	assert.Equal(t, common.Running, runner.Status)
}
