package run_test

import (
	"context"
	"testing"

	runcommand "github.com/gruntwork-io/terragrunt/internal/cli/commands/run"
	"github.com/gruntwork-io/terragrunt/internal/clihelper"
	"github.com/gruntwork-io/terragrunt/internal/experiment"
	"github.com/gruntwork-io/terragrunt/internal/report"
	"github.com/gruntwork-io/terragrunt/internal/strict/controls"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/pkg/options"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNoHooksFlagSetsNoRunHooks(t *testing.T) {
	t.Parallel()

	opts := options.NewTerragruntOptions()
	flags := runcommand.NewFlags(logger.CreateLogger(), opts, venv.MemVenv(), nil)

	require.NoError(t, flags.Parse(clihelper.Args{"--no-hooks"}, map[string]string{}))
	require.NoError(t, flags.RunActions(context.Background(), &clihelper.Context{}))
	assert.True(t, opts.NoRunHooks)
}

func TestNoDependencyOutputsFlagSetsSkipOutput(t *testing.T) {
	t.Parallel()

	opts := options.NewTerragruntOptions()
	flags := runcommand.NewFlags(logger.CreateLogger(), opts, venv.MemVenv(), nil)

	require.NoError(t, flags.Parse(clihelper.Args{"--no-dependency-outputs"}, map[string]string{}))
	require.NoError(t, flags.RunActions(context.Background(), &clihelper.Context{}))
	assert.True(t, opts.SkipOutput)
}

// TestNewFlagsDeprecatedFlagsRunTheirStrictControls pins that each deprecated flag fails once its control is on.
func TestNewFlagsDeprecatedFlagsRunTheirStrictControls(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		control string
		args    clihelper.Args
	}{
		{
			name:    "dependency-fetch-output-from-state",
			control: controls.DependencyFetchOutputFromState,
			args:    clihelper.Args{"--" + runcommand.DependencyFetchOutputFromStateFlagName},
		},
		{
			name:    "units-that-include",
			control: controls.UnitsThatInclude,
			args:    clihelper.Args{"--" + runcommand.UnitsThatIncludeFlagName, "root.hcl"},
		},
		{
			name:    "disable-command-validation",
			control: controls.DisableCommandValidation,
			args:    clihelper.Args{"--" + runcommand.DisableCommandValidationFlagName},
		},
		{
			name:    "no-destroy-dependencies-check",
			control: controls.NoDestroyDependenciesCheck,
			args:    clihelper.Args{"--" + runcommand.NoDestroyDependenciesCheckFlagName},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			for _, strictMode := range []bool{false, true} {
				opts := options.NewTerragruntOptions()
				if strictMode {
					require.NoError(t, opts.StrictControls.EnableControl(tc.control))
				}

				err := parseRunFlags(t, opts, tc.args)
				if strictMode {
					require.Error(t, err, "the flag fails under its strict control")

					continue
				}

				require.NoError(t, err, "the flag only warns without its strict control")
			}
		})
	}
}

// TestNewFlagsBoolFlagsSetToFalseSkipTheirStrictControls pins that a deprecated flag set to false is not a use of it.
func TestNewFlagsBoolFlagsSetToFalseSkipTheirStrictControls(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		control string
		flag    string
	}{
		{
			name:    "dependency-fetch-output-from-state",
			control: controls.DependencyFetchOutputFromState,
			flag:    runcommand.DependencyFetchOutputFromStateFlagName,
		},
		{
			name:    "disable-command-validation",
			control: controls.DisableCommandValidation,
			flag:    runcommand.DisableCommandValidationFlagName,
		},
		{
			name:    "no-destroy-dependencies-check",
			control: controls.NoDestroyDependenciesCheck,
			flag:    runcommand.NoDestroyDependenciesCheckFlagName,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			opts := options.NewTerragruntOptions()
			require.NoError(t, opts.StrictControls.EnableControl(tc.control))

			require.NoError(t, parseRunFlags(t, opts, clihelper.Args{"--" + tc.flag + "=false"}))
		})
	}
}

// TestNewFlagsUnitsThatIncludeAddsReadingFilters pins the filters --units-that-include turns into.
func TestNewFlagsUnitsThatIncludeAddsReadingFilters(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name        string
		args        clihelper.Args
		wantFilters []string
		wantErr     bool
	}{
		{name: "unset adds no filter"},
		{
			name: "every value becomes a reading filter",
			args: clihelper.Args{
				"--" + runcommand.UnitsThatIncludeFlagName, "root.hcl",
				"--" + runcommand.UnitsThatIncludeFlagName, "common/*.hcl",
			},
			wantFilters: []string{"reading=root.hcl", "reading=common/*.hcl"},
		},
		{
			name:    "an invalid glob is rejected",
			args:    clihelper.Args{"--" + runcommand.UnitsThatIncludeFlagName, "[unclosed"},
			wantErr: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			opts := options.NewTerragruntOptions()

			err := parseRunFlags(t, opts, tc.args)
			if tc.wantErr {
				require.Error(t, err)
				assert.Empty(t, opts.Filters, "no filter is added for an invalid glob")

				return
			}

			require.NoError(t, err)

			var filters []string

			for _, f := range opts.Filters {
				filters = append(filters, f.String())
			}

			assert.Equal(t, tc.wantFilters, filters)
		})
	}
}

// TestNewFlagsExperimentalEngineEnablesTheExperiment pins that --experimental-engine turns on the iac-engine experiment.
func TestNewFlagsExperimentalEngineEnablesTheExperiment(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		args clihelper.Args
		want bool
	}{
		{name: "unset"},
		{name: "set", args: clihelper.Args{"--" + runcommand.EngineEnableFlagName}, want: true},
		{name: "set to false", args: clihelper.Args{"--" + runcommand.EngineEnableFlagName + "=false"}},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			opts := options.NewTerragruntOptions()

			require.NoError(t, parseRunFlags(t, opts, tc.args))
			assert.Equal(t, tc.want, opts.Experiments.Evaluate(experiment.IacEngine))
		})
	}
}

// TestNewFlagsReportFlagsSetTheReportFormat pins the report file and format the report flags resolve to.
func TestNewFlagsReportFlagsSetTheReportFormat(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name       string
		wantFile   string
		wantFormat report.Format
		args       clihelper.Args
		wantErr    bool
	}{
		{name: "unset"},
		{name: "empty report file is ignored", args: clihelper.Args{"--report-file="}},
		{
			name:       "json extension picks json",
			args:       clihelper.Args{"--report-file", "report.json"},
			wantFile:   "report.json",
			wantFormat: report.FormatJSON,
		},
		{
			name:       "no extension picks csv",
			args:       clihelper.Args{"--report-file", "report"},
			wantFile:   "report",
			wantFormat: report.FormatCSV,
		},
		{
			name:     "unknown extension picks no format",
			args:     clihelper.Args{"--report-file", "report.txt"},
			wantFile: "report.txt",
		},
		{
			name:       "explicit format wins over the extension",
			args:       clihelper.Args{"--report-format=json", "--report-file=report.csv"},
			wantFile:   "report.csv",
			wantFormat: report.FormatJSON,
		},
		{
			name:       "empty format defaults to csv",
			args:       clihelper.Args{"--report-format="},
			wantFormat: report.FormatCSV,
		},
		{
			name:       "csv format",
			args:       clihelper.Args{"--report-format", "csv"},
			wantFormat: report.FormatCSV,
		},
		{
			name:    "unsupported format is rejected",
			args:    clihelper.Args{"--report-format", "yaml"},
			wantErr: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			opts := options.NewTerragruntOptions()

			err := parseRunFlags(t, opts, tc.args)
			if tc.wantErr {
				require.Error(t, err)

				return
			}

			require.NoError(t, err)
			assert.Equal(t, tc.wantFile, opts.ReportFile)
			assert.Equal(t, tc.wantFormat, opts.ReportFormat)
		})
	}
}

// parseRunFlags parses args into the run flags bound to opts and runs their actions.
func parseRunFlags(t *testing.T, opts *options.TerragruntOptions, args clihelper.Args) error {
	t.Helper()

	flags := runcommand.NewFlags(logger.CreateLogger(), opts, venv.MemVenv(), nil)

	if err := flags.Parse(args, map[string]string{}); err != nil {
		return err
	}

	return flags.RunActions(t.Context(), &clihelper.Context{})
}
