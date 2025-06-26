package runbase

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/report"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"

	"github.com/stretchr/testify/assert"
)

// mockUnit is a minimal mock for Unit to test UnitRunner logic
// You may want to expand this for more complex tests
func newMockUnit() *Unit {
	return &Unit{
		Logger:            logger.CreateLogger(),
		Path:              "mock/path",
		TerragruntOptions: &options.TerragruntOptions{},
	}
}

func TestNewUnitRunner(t *testing.T) {
	t.Parallel()
	unit := newMockUnit()
	runner := NewUnitRunner(unit)
	assert.Equal(t, unit, runner.Unit)
	assert.Equal(t, Waiting, runner.Status)
}

func TestUnitRunner_Run_AssumeAlreadyApplied(t *testing.T) {
	t.Parallel()

	unit := newMockUnit()
	unit.AssumeAlreadyApplied = true
	runner := NewUnitRunner(unit)
	report := &report.Report{}
	err := runner.Run(t.Context(), &options.TerragruntOptions{}, report)
	assert.NoError(t, err)
	assert.Equal(t, Running, runner.Status)
}

func TestUnitRunner_Run_ErrorFromRunTerragrunt(t *testing.T) {
	t.Parallel()

	unit := newMockUnit()
	unit.TerragruntOptions = &options.TerragruntOptions{
		Writer: &bytes.Buffer{},
		RunTerragrunt: func(ctx context.Context, l log.Logger, opts *options.TerragruntOptions, r *report.Report) error {
			return errors.New("fail")
		},
	}
	runner := NewUnitRunner(unit)
	report := &report.Report{}
	err := runner.Run(t.Context(), &options.TerragruntOptions{Writer: &bytes.Buffer{}}, report)
	assert.Error(t, err)
	assert.Equal(t, Running, runner.Status)
	assert.Contains(t, err.Error(), "fail")
}

func TestUnitRunner_Run_Success(t *testing.T) {
	t.Parallel()

	unit := newMockUnit()
	unit.TerragruntOptions = &options.TerragruntOptions{
		Writer: &bytes.Buffer{},
		RunTerragrunt: func(ctx context.Context, l log.Logger, opts *options.TerragruntOptions, r *report.Report) error {
			return nil
		},
	}
	runner := NewUnitRunner(unit)
	report := &report.Report{}
	err := runner.Run(t.Context(), &options.TerragruntOptions{Writer: &bytes.Buffer{}}, report)
	assert.NoError(t, err)
	assert.Equal(t, Running, runner.Status)
}
