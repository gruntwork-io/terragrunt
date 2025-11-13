package dto

import (
	"errors"
	"testing"
	"time"

	"github.com/gruntwork-io/terragrunt/config"
	"github.com/gruntwork-io/terragrunt/internal/component"
	"github.com/gruntwork-io/terragrunt/internal/filter"
	"github.com/stretchr/testify/assert"
)

func TestNewUnitDiscoveryDTO(t *testing.T) {
	t.Parallel()

	path := "/test/path"
	dto := NewUnitDiscoveryDTO(path)

	assert.Equal(t, path, dto.Path)
	assert.Equal(t, component.UnitKind, dto.Kind)
	assert.Empty(t, dto.Reading)
	assert.Empty(t, dto.Sources)
	assert.Empty(t, dto.DependencyPaths)
	assert.Empty(t, dto.DependentPaths)
	assert.Empty(t, dto.ParseErrors)
	assert.False(t, dto.IsExternal)
	assert.False(t, dto.FilterExcluded)
	assert.False(t, dto.RequiresApply)
}

func TestUnitDiscoveryDTO_FluentAPI(t *testing.T) {
	t.Parallel()

	cfg := &config.TerragruntConfig{}
	ctx := &component.DiscoveryContext{}
	parseErr := errors.New("parse error")

	dto := NewUnitDiscoveryDTO("/test/path").
		WithConfig(cfg).
		WithReading("file1.hcl", "file2.hcl").
		WithSources("source1", "source2").
		WithDiscoveryContext(ctx).
		WithDependencyPaths("/dep1", "/dep2").
		WithDependentPaths("/dependent1").
		MarkExternal().
		MarkFilterExcluded().
		MarkRequiresApply().
		AddParseError(parseErr)

	assert.Equal(t, cfg, dto.Config)
	assert.Equal(t, []string{"file1.hcl", "file2.hcl"}, dto.Reading)
	assert.Equal(t, []string{"source1", "source2"}, dto.Sources)
	assert.Equal(t, ctx, dto.DiscoveryContext)
	assert.Equal(t, []string{"/dep1", "/dep2"}, dto.DependencyPaths)
	assert.Equal(t, []string{"/dependent1"}, dto.DependentPaths)
	assert.True(t, dto.IsExternal)
	assert.True(t, dto.FilterExcluded)
	assert.True(t, dto.RequiresApply)
	assert.Contains(t, dto.ParseErrors, parseErr)
}

func TestUnitDiscoveryDTO_AddParseError_IgnoresNil(t *testing.T) {
	t.Parallel()

	dto := NewUnitDiscoveryDTO("/test/path").AddParseError(nil)

	assert.Empty(t, dto.ParseErrors)
}

func TestNewStackDiscoveryDTO(t *testing.T) {
	t.Parallel()

	path := "/test/stack"
	dto := NewStackDiscoveryDTO(path)

	assert.Equal(t, path, dto.Path)
	assert.Empty(t, dto.Reading)
	assert.Empty(t, dto.ParseErrors)
	assert.False(t, dto.IsExternal)
	assert.False(t, dto.FilterExcluded)
}

func TestStackDiscoveryDTO_FluentAPI(t *testing.T) {
	t.Parallel()

	cfg := &config.StackConfig{}
	ctx := &component.DiscoveryContext{}
	parseErr := errors.New("parse error")

	dto := NewStackDiscoveryDTO("/test/stack").
		WithConfig(cfg).
		WithReading("stack.hcl").
		WithDiscoveryContext(ctx).
		MarkExternal().
		MarkFilterExcluded().
		AddParseError(parseErr)

	assert.Equal(t, cfg, dto.Config)
	assert.Equal(t, []string{"stack.hcl"}, dto.Reading)
	assert.Equal(t, ctx, dto.DiscoveryContext)
	assert.True(t, dto.IsExternal)
	assert.True(t, dto.FilterExcluded)
	assert.Contains(t, dto.ParseErrors, parseErr)
}

func TestNewDiscoveryRequestDTO(t *testing.T) {
	t.Parallel()

	workingDir := "/working/dir"
	dto := NewDiscoveryRequestDTO(workingDir)

	assert.Equal(t, workingDir, dto.WorkingDir)
	assert.Nil(t, dto.ConfigFilenames)
	assert.Empty(t, dto.IncludeDirs)
	assert.Empty(t, dto.ExcludeDirs)
	assert.Empty(t, dto.Filters)
	assert.Empty(t, dto.ParserOptions)
	assert.True(t, dto.DiscoverDependencies)
	assert.False(t, dto.ExcludeByDefault)
	assert.False(t, dto.NoHidden)
	assert.False(t, dto.RequiresParse)
	assert.False(t, dto.StrictInclude)
	assert.False(t, dto.ParseExclude)
	assert.False(t, dto.ParseInclude)
	assert.Equal(t, 0, dto.NumWorkers)
	assert.Equal(t, 0, dto.MaxDependencyDepth)
	assert.Empty(t, dto.Sort)
}

func TestDiscoveryRequestDTO_FluentAPI(t *testing.T) {
	t.Parallel()

	ctx := &component.DiscoveryContext{}
	filters := filter.Filters{}

	dto := NewDiscoveryRequestDTO("/working").
		WithDiscoveryContext(ctx).
		WithConfigFilenames("terragrunt.hcl").
		WithIncludeDirs("include/*").
		WithExcludeDirs("exclude/*").
		WithFilters(filters).
		WithDiscoverDependencies(false).
		WithNumWorkers(8).
		WithMaxDependencyDepth(100).
		WithSort("name").
		EnableStrictInclude().
		EnableExcludeByDefault().
		EnableNoHidden().
		EnableRequiresParse().
		EnableParseExclude().
		EnableParseInclude()

	assert.Equal(t, ctx, dto.DiscoveryContext)
	assert.Equal(t, []string{"terragrunt.hcl"}, dto.ConfigFilenames)
	assert.Equal(t, []string{"include/*"}, dto.IncludeDirs)
	assert.Equal(t, []string{"exclude/*"}, dto.ExcludeDirs)
	assert.Equal(t, filters, dto.Filters)
	assert.False(t, dto.DiscoverDependencies)
	assert.Equal(t, 8, dto.NumWorkers)
	assert.Equal(t, 100, dto.MaxDependencyDepth)
	assert.Equal(t, "name", dto.Sort)
	assert.True(t, dto.StrictInclude)
	assert.True(t, dto.ExcludeByDefault)
	assert.True(t, dto.NoHidden)
	assert.True(t, dto.RequiresParse)
	assert.True(t, dto.ParseExclude)
	assert.True(t, dto.ParseInclude)
}

func TestNewDiscoveryResultDTO(t *testing.T) {
	t.Parallel()

	dto := NewDiscoveryResultDTO()

	assert.Empty(t, dto.Units)
	assert.Empty(t, dto.Stacks)
	assert.Empty(t, dto.Relationships)
	assert.Empty(t, dto.Errors)
	assert.NotNil(t, dto.Metadata)
	assert.Equal(t, 0, dto.Metadata.TotalUnitsFound)
	assert.Equal(t, 0, dto.Metadata.TotalStacksFound)
	assert.Equal(t, 0, dto.Metadata.UnitsExcluded)
	assert.Equal(t, 0, dto.Metadata.StacksExcluded)
	assert.Equal(t, 0, dto.Metadata.ParseErrors)
	assert.Equal(t, 0, dto.Metadata.DependenciesDiscovered)
}

func TestDiscoveryResultDTO_AddUnit(t *testing.T) {
	t.Parallel()

	result := NewDiscoveryResultDTO()

	unit1 := NewUnitDiscoveryDTO("/unit1")
	unit2 := NewUnitDiscoveryDTO("/unit2").MarkFilterExcluded()
	unit3 := NewUnitDiscoveryDTO("/unit3").AddParseError(errors.New("error"))

	result.AddUnit(unit1).AddUnit(unit2).AddUnit(unit3)

	assert.Len(t, result.Units, 3)
	assert.Equal(t, 3, result.Metadata.TotalUnitsFound)
	assert.Equal(t, 1, result.Metadata.UnitsExcluded)
	assert.Equal(t, 1, result.Metadata.ParseErrors)
}

func TestDiscoveryResultDTO_AddStack(t *testing.T) {
	t.Parallel()

	result := NewDiscoveryResultDTO()

	stack1 := NewStackDiscoveryDTO("/stack1")
	stack2 := NewStackDiscoveryDTO("/stack2").MarkFilterExcluded()

	result.AddStack(stack1).AddStack(stack2)

	assert.Len(t, result.Stacks, 2)
	assert.Equal(t, 2, result.Metadata.TotalStacksFound)
	assert.Equal(t, 1, result.Metadata.StacksExcluded)
}

func TestDiscoveryResultDTO_AddRelationship(t *testing.T) {
	t.Parallel()

	result := NewDiscoveryResultDTO()

	result.AddRelationship("/unit1", "/dep1", "/dep2")
	result.AddRelationship("/unit2", "/dep3")

	assert.Len(t, result.Relationships, 2)
	assert.Equal(t, []string{"/dep1", "/dep2"}, result.Relationships["/unit1"])
	assert.Equal(t, []string{"/dep3"}, result.Relationships["/unit2"])
	assert.Equal(t, 3, result.Metadata.DependenciesDiscovered)
}

func TestDiscoveryResultDTO_GetIncludedUnits(t *testing.T) {
	t.Parallel()

	result := NewDiscoveryResultDTO()
	unit1 := NewUnitDiscoveryDTO("/unit1")
	unit2 := NewUnitDiscoveryDTO("/unit2").MarkFilterExcluded()
	unit3 := NewUnitDiscoveryDTO("/unit3")

	result.AddUnit(unit1).AddUnit(unit2).AddUnit(unit3)

	included := result.GetIncludedUnits()

	assert.Len(t, included, 2)
	assert.Contains(t, included, unit1)
	assert.Contains(t, included, unit3)
	assert.NotContains(t, included, unit2)
}

func TestDiscoveryResultDTO_GetIncludedStacks(t *testing.T) {
	t.Parallel()

	result := NewDiscoveryResultDTO()
	stack1 := NewStackDiscoveryDTO("/stack1")
	stack2 := NewStackDiscoveryDTO("/stack2").MarkFilterExcluded()

	result.AddStack(stack1).AddStack(stack2)

	included := result.GetIncludedStacks()

	assert.Len(t, included, 1)
	assert.Contains(t, included, stack1)
	assert.NotContains(t, included, stack2)
}

func TestDiscoveryResultDTO_GetUnitByPath(t *testing.T) {
	t.Parallel()

	result := NewDiscoveryResultDTO()
	unit1 := NewUnitDiscoveryDTO("/unit1")
	unit2 := NewUnitDiscoveryDTO("/unit2")

	result.AddUnit(unit1).AddUnit(unit2)

	found := result.GetUnitByPath("/unit1")
	assert.Equal(t, unit1, found)

	notFound := result.GetUnitByPath("/nonexistent")
	assert.Nil(t, notFound)
}

func TestDiscoveryResultDTO_GetStackByPath(t *testing.T) {
	t.Parallel()

	result := NewDiscoveryResultDTO()
	stack := NewStackDiscoveryDTO("/stack1")

	result.AddStack(stack)

	found := result.GetStackByPath("/stack1")
	assert.Equal(t, stack, found)

	notFound := result.GetStackByPath("/nonexistent")
	assert.Nil(t, notFound)
}

func TestDiscoveryResultDTO_HasErrors(t *testing.T) {
	t.Parallel()

	t.Run("no errors", func(t *testing.T) {
		result := NewDiscoveryResultDTO()
		assert.False(t, result.HasErrors())
	})

	t.Run("discovery error", func(t *testing.T) {
		result := NewDiscoveryResultDTO()
		result.AddError(errors.New("error"))
		assert.True(t, result.HasErrors())
	})

	t.Run("parse error", func(t *testing.T) {
		result := NewDiscoveryResultDTO()
		unit := NewUnitDiscoveryDTO("/unit").AddParseError(errors.New("parse error"))
		result.AddUnit(unit)
		assert.True(t, result.HasErrors())
	})
}

func TestNewExecutionResultDTO(t *testing.T) {
	t.Parallel()

	path := "/unit/path"
	dto := NewExecutionResultDTO(path)

	assert.Equal(t, path, dto.UnitPath)
	assert.Equal(t, ExecutionStatusPending, dto.Status)
	assert.NotNil(t, dto.Metadata)
	assert.True(t, dto.StartTime.IsZero())
	assert.True(t, dto.EndTime.IsZero())
}

func TestExecutionResultDTO_FluentAPI(t *testing.T) {
	t.Parallel()

	start := time.Now()
	end := start.Add(5 * time.Second)

	dto := NewExecutionResultDTO("/unit").
		WithStartTime(start).
		WithEndTime(end).
		WithOutput("test output").
		WithError(errors.New("test error")). // WithError sets status to failed when status is pending
		WithExitCode(1).
		AddMetadata("key", "value")

	assert.Equal(t, ExecutionStatusFailed, dto.Status) // WithError sets to failed when status is pending
	assert.Equal(t, start, dto.StartTime)
	assert.Equal(t, end, dto.EndTime)
	assert.Equal(t, "test output", dto.Output)
	assert.NotNil(t, dto.Error)
	assert.Equal(t, 1, dto.ExitCode)
	assert.Equal(t, "value", dto.Metadata["key"])
}

func TestExecutionResultDTO_Duration(t *testing.T) {
	t.Parallel()

	dto := NewExecutionResultDTO("/unit")

	// Zero when times not set
	assert.Equal(t, time.Duration(0), dto.Duration())

	// Calculate duration when set
	start := time.Now()
	end := start.Add(5 * time.Second)
	dto.WithStartTime(start).WithEndTime(end)

	assert.Equal(t, 5*time.Second, dto.Duration())
}

func TestExecutionResultDTO_StatusChecks(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		status     ExecutionStatus
		isSuccess  bool
		isFailed   bool
		isComplete bool
	}{
		{"pending", ExecutionStatusPending, false, false, false},
		{"running", ExecutionStatusRunning, false, false, false},
		{"success", ExecutionStatusSuccess, true, false, true},
		{"failed", ExecutionStatusFailed, false, true, true},
		{"skipped", ExecutionStatusSkipped, false, false, true},
		{"cancelled", ExecutionStatusCancelled, false, false, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dto := NewExecutionResultDTO("/unit").WithStatus(tt.status)

			assert.Equal(t, tt.isSuccess, dto.IsSuccess())
			assert.Equal(t, tt.isFailed, dto.IsFailed())
			assert.Equal(t, tt.isComplete, dto.IsComplete())
		})
	}
}

func TestExecutionResultDTO_MarkMethods(t *testing.T) {
	t.Parallel()

	t.Run("MarkStarted", func(t *testing.T) {
		dto := NewExecutionResultDTO("/unit").MarkStarted()

		assert.Equal(t, ExecutionStatusRunning, dto.Status)
		assert.False(t, dto.StartTime.IsZero())
	})

	t.Run("MarkSuccess", func(t *testing.T) {
		dto := NewExecutionResultDTO("/unit").MarkSuccess()

		assert.Equal(t, ExecutionStatusSuccess, dto.Status)
		assert.False(t, dto.EndTime.IsZero())
	})

	t.Run("MarkFailed", func(t *testing.T) {
		err := errors.New("test error")
		dto := NewExecutionResultDTO("/unit").MarkFailed(err)

		assert.Equal(t, ExecutionStatusFailed, dto.Status)
		assert.Equal(t, err, dto.Error)
		assert.False(t, dto.EndTime.IsZero())
	})

	t.Run("MarkSkipped", func(t *testing.T) {
		dto := NewExecutionResultDTO("/unit").MarkSkipped()

		assert.Equal(t, ExecutionStatusSkipped, dto.Status)
		assert.False(t, dto.EndTime.IsZero())
	})

	t.Run("MarkCancelled", func(t *testing.T) {
		dto := NewExecutionResultDTO("/unit").MarkCancelled()

		assert.Equal(t, ExecutionStatusCancelled, dto.Status)
		assert.False(t, dto.EndTime.IsZero())
	})
}

func TestNewExecutionBatchResultDTO(t *testing.T) {
	t.Parallel()

	dto := NewExecutionBatchResultDTO()

	assert.Empty(t, dto.Results)
	assert.NotNil(t, dto.Summary)
	assert.Equal(t, 0, dto.Summary.TotalUnits)
}

func TestExecutionBatchResultDTO_AddResult(t *testing.T) {
	t.Parallel()

	batch := NewExecutionBatchResultDTO()

	result1 := NewExecutionResultDTO("/unit1").MarkSuccess()
	result2 := NewExecutionResultDTO("/unit2").MarkFailed(errors.New("error"))
	result3 := NewExecutionResultDTO("/unit3").MarkSkipped()

	batch.AddResult(result1).AddResult(result2).AddResult(result3)

	assert.Len(t, batch.Results, 3)
	assert.Equal(t, 3, batch.Summary.TotalUnits)
	assert.Equal(t, 1, batch.Summary.SuccessCount)
	assert.Equal(t, 1, batch.Summary.FailedCount)
	assert.Equal(t, 1, batch.Summary.SkippedCount)
}

func TestExecutionBatchResultDTO_HasFailures(t *testing.T) {
	t.Parallel()

	batch := NewExecutionBatchResultDTO()
	assert.False(t, batch.HasFailures())

	batch.AddResult(NewExecutionResultDTO("/unit").MarkFailed(errors.New("error")))
	assert.True(t, batch.HasFailures())
}

func TestExecutionBatchResultDTO_AllSuccessful(t *testing.T) {
	t.Parallel()

	batch := NewExecutionBatchResultDTO()
	assert.False(t, batch.AllSuccessful()) // No results

	batch.AddResult(NewExecutionResultDTO("/unit1").MarkSuccess())
	assert.True(t, batch.AllSuccessful())

	batch.AddResult(NewExecutionResultDTO("/unit2").MarkFailed(errors.New("error")))
	assert.False(t, batch.AllSuccessful())
}

func TestDiscoveryResultDTO_WithWorkingDir(t *testing.T) {
	t.Parallel()

	result := NewDiscoveryResultDTO().WithWorkingDir("/working/dir")

	assert.Equal(t, "/working/dir", result.Metadata.WorkingDir)
}

func TestDiscoveryResultDTO_AddError_IgnoresNil(t *testing.T) {
	t.Parallel()

	result := NewDiscoveryResultDTO().AddError(nil)

	assert.Empty(t, result.Errors)
}
