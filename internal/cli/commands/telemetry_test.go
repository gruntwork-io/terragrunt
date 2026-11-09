package commands_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/cli/commands"
	"github.com/gruntwork-io/terragrunt/internal/cli/commands/run"
	"github.com/gruntwork-io/terragrunt/internal/clihelper"
	"github.com/gruntwork-io/terragrunt/internal/filter"
	"github.com/gruntwork-io/terragrunt/internal/telemetry"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/internal/vexec"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/pkg/options"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var errActionFailed = errors.New("action failed")

// TestWrapWithTelemetryResolvesPaths pins how the wrapper makes the working dir, download dir, config path, and tf path absolute.
func TestWrapWithTelemetryResolvesPaths(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name           string
		cmdName        string
		workingDir     string
		downloadDir    string
		configPath     string
		tfPath         string
		wantWorkingDir string
		wantDownload   string
		wantConfig     string
		wantTFPath     string
	}{
		{
			name:           "defaults under the working dir",
			cmdName:        run.CommandName,
			workingDir:     hostPath("/work/nested/.."),
			tfPath:         "tofu",
			wantWorkingDir: testWorkDir,
			wantDownload:   filepath.Join(testWorkDir, ".terragrunt-cache"),
			wantConfig:     filepath.Join(testWorkDir, "terragrunt.hcl"),
			wantTFPath:     "tofu",
		},
		{
			name:           "relative paths join the working dir for run",
			cmdName:        run.CommandName,
			workingDir:     testWorkDir,
			downloadDir:    "cache",
			configPath:     "units/app.hcl",
			tfPath:         filepath.Join("bin", "tofu"),
			wantWorkingDir: testWorkDir,
			wantDownload:   filepath.Join(testWorkDir, "cache"),
			wantConfig:     filepath.Join(testWorkDir, "units", "app.hcl"),
			wantTFPath:     filepath.Join(testWorkDir, "bin", "tofu"),
		},
		{
			name:           "absolute paths are only cleaned",
			cmdName:        run.CommandName,
			workingDir:     testWorkDir,
			downloadDir:    hostPath("/cache/./dl"),
			configPath:     hostPath("/configs/app.hcl"),
			tfPath:         hostPath("/usr/bin/tofu"),
			wantWorkingDir: testWorkDir,
			wantDownload:   hostPath("/cache/dl"),
			wantConfig:     hostPath("/configs/app.hcl"),
			wantTFPath:     hostPath("/usr/bin/tofu"),
		},
		{
			name:           "relative config path stays relative outside run",
			cmdName:        "find",
			workingDir:     testWorkDir,
			configPath:     "./units/app.hcl",
			tfPath:         "tofu",
			wantWorkingDir: testWorkDir,
			wantDownload:   filepath.Join(testWorkDir, ".terragrunt-cache"),
			wantConfig:     filepath.Join("units", "app.hcl"),
			wantTFPath:     "tofu",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			opts := newWrapTestOptions()
			opts.WorkingDir = tc.workingDir
			opts.DownloadDir = tc.downloadDir
			opts.TerragruntConfigPath = tc.configPath
			opts.TFPath = tc.tfPath

			app, _ := newWrapTestApp()
			cliCtx := newCommandContext(app, clihelper.Args{"plan"}, &clihelper.Command{Name: tc.cmdName})

			require.NoError(t, runWithTelemetry(t, logger.CreateLogger(), opts, venv.MemVenv(), cliCtx, nil))
			assert.Equal(t, tc.wantWorkingDir, opts.WorkingDir, "working dir")
			assert.Equal(t, tc.wantWorkingDir, opts.RootWorkingDir, "root working dir should match working dir")
			assert.Equal(t, tc.wantDownload, opts.DownloadDir, "download dir")
			assert.Equal(t, tc.wantConfig, opts.TerragruntConfigPath, "config path")
			assert.Equal(t, tc.wantConfig, opts.OriginalTerragruntConfigPath, "original config path")
			assert.Equal(t, tc.wantTFPath, opts.TFPath, "tf path")
		})
	}
}

// TestWrapWithTelemetryWorkingDirFallbacks pins that an unset working dir uses the current dir and a relative one becomes absolute.
func TestWrapWithTelemetryWorkingDirFallbacks(t *testing.T) {
	t.Parallel()

	t.Run("unset uses current dir", func(t *testing.T) {
		t.Parallel()

		v := venv.MemVenv()

		wantDir, err := v.Platform.Getwd()
		require.NoError(t, err)

		opts := newWrapTestOptions()
		app, _ := newWrapTestApp()
		cliCtx := newCommandContext(app, nil, &clihelper.Command{Name: "find"})

		require.NoError(t, runWithTelemetry(t, logger.CreateLogger(), opts, v, cliCtx, nil))
		assert.Equal(t, filepath.Clean(wantDir), opts.WorkingDir)
	})

	t.Run("relative becomes absolute", func(t *testing.T) {
		t.Parallel()

		opts := newWrapTestOptions()
		opts.WorkingDir = "units/app"

		app, _ := newWrapTestApp()
		cliCtx := newCommandContext(app, nil, &clihelper.Command{Name: "find"})

		require.NoError(t, runWithTelemetry(t, logger.CreateLogger(), opts, venv.MemVenv(), cliCtx, nil))
		assert.True(t, filepath.IsAbs(opts.WorkingDir), "working dir %q should be absolute", opts.WorkingDir)
		assert.True(
			t,
			strings.HasSuffix(opts.WorkingDir, filepath.Join("units", "app")),
			"working dir %q should end with the relative path",
			opts.WorkingDir,
		)
	})

	t.Run("current dir error fails the command", func(t *testing.T) {
		t.Parallel()

		v := venv.MemVenv()
		v.Platform.Getwd = func() (string, error) { return "", errInjected }

		actionRan := false

		opts := newWrapTestOptions()
		app, _ := newWrapTestApp()
		cliCtx := newCommandContext(app, nil, &clihelper.Command{Name: "find"})

		err := runWithTelemetry(t, logger.CreateLogger(), opts, v, cliCtx, func(context.Context, *clihelper.Context) error {
			actionRan = true

			return nil
		})
		require.ErrorIs(t, err, errInjected)
		assert.False(t, actionRan, "the action must not run without a working dir")
	})
}

// TestWrapWithTelemetryBuildsTofuArgs pins the OpenTofu command and arguments the wrapper derives from the CLI args.
func TestWrapWithTelemetryBuildsTofuArgs(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name            string
		cmdName         string
		args            clihelper.Args
		wantTofuCommand string
		wantArgs        []string
		noColor         bool
	}{
		{
			name:            "run takes the command from its first arg",
			cmdName:         run.CommandName,
			args:            clihelper.Args{"plan", "--input=false"},
			wantTofuCommand: "plan",
			wantArgs:        []string{"plan", "-input=false"},
		},
		{
			name:            "other commands prepend their own name",
			cmdName:         "plan",
			args:            clihelper.Args{"--out=plan.tfplan"},
			wantTofuCommand: "plan",
			wantArgs:        []string{"plan", "-out=plan.tfplan"},
		},
		{
			name:            "disabled colors add no color",
			cmdName:         run.CommandName,
			args:            clihelper.Args{"apply"},
			noColor:         true,
			wantTofuCommand: "apply",
			wantArgs:        []string{"apply", "-no-color"},
		},
		{
			name:            "explicit no color is not repeated",
			cmdName:         run.CommandName,
			args:            clihelper.Args{"apply", "-no-color"},
			noColor:         true,
			wantTofuCommand: "apply",
			wantArgs:        []string{"apply", "-no-color"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			l := logger.CreateLogger()
			l.Formatter().SetDisabledColors(tc.noColor)

			opts := newWrapTestOptions()
			opts.WorkingDir = testWorkDir

			app, _ := newWrapTestApp()
			cliCtx := newCommandContext(app, tc.args, &clihelper.Command{Name: tc.cmdName})

			require.NoError(t, runWithTelemetry(t, l, opts, venv.MemVenv(), cliCtx, nil))
			assert.Equal(t, tc.wantTofuCommand, opts.TofuCommand, "tofu command")
			assert.Equal(t, tc.wantTofuCommand, opts.OriginalTofuCommand, "original tofu command")
			assert.Equal(t, tc.wantArgs, opts.TerraformCliArgs.Slice(), "tofu args")
		})
	}
}

// TestWrapWithTelemetryReadsFilterFiles pins how the excludes and filters files add deduplicated filters.
func TestWrapWithTelemetryReadsFilterFiles(t *testing.T) {
	t.Parallel()

	excludesFile := filepath.Join(testWorkDir, ".terragrunt-excludes")
	filtersFile := filepath.Join(testWorkDir, ".terragrunt-filters")

	testCases := []struct {
		files         map[string]string
		name          string
		preset        []string
		wantFilters   []string
		noFiltersFile bool
	}{
		{
			name:        "excludes file negates each dir",
			files:       map[string]string{excludesFile: "# skip\nlegacy\n\nold/**\n"},
			wantFilters: []string{"!legacy", "!old/**"},
		},
		{
			name:        "filters file adds each query",
			files:       map[string]string{filtersFile: "./apps/*\n# comment\n./db\n"},
			wantFilters: []string{"./apps/*", "./db"},
		},
		{
			name:          "filters file is skipped when disabled",
			files:         map[string]string{filtersFile: "./apps/*\n"},
			noFiltersFile: true,
			wantFilters:   []string{},
		},
		{
			name:        "duplicates of flag filters are dropped",
			files:       map[string]string{filtersFile: "./apps/*\n./db\n"},
			preset:      []string{"./db"},
			wantFilters: []string{"./db", "./apps/*"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			v := venv.MemVenv()
			for path, content := range tc.files {
				require.NoError(t, vfs.WriteFile(v.FS, path, []byte(content), 0o644))
			}

			opts := newWrapTestOptions()
			opts.WorkingDir = testWorkDir
			opts.NoFiltersFile = tc.noFiltersFile
			opts.Filters = mustParseFilters(t, tc.preset)

			app, _ := newWrapTestApp()
			cliCtx := newCommandContext(app, nil, &clihelper.Command{Name: "find"})

			require.NoError(t, runWithTelemetry(t, logger.CreateLogger(), opts, v, cliCtx, nil))
			assert.Equal(t, tc.wantFilters, filterQueries(opts.Filters), "filters after setup")
		})
	}
}

// TestWrapWithTelemetryFilterFileErrors pins that an unreadable or malformed filter file fails the command before its action.
func TestWrapWithTelemetryFilterFileErrors(t *testing.T) {
	t.Parallel()

	excludesFile := filepath.Join(testWorkDir, ".terragrunt-excludes")
	filtersFile := filepath.Join(testWorkDir, ".terragrunt-filters")

	testCases := []struct {
		wantErr error
		fsys    func(base vfs.FS) vfs.FS
		files   map[string]string
		name    string
	}{
		{
			name:  "unreadable excludes file",
			files: map[string]string{excludesFile: "legacy\n"},
			fsys: func(base vfs.FS) vfs.FS {
				return newFaultFS(base, atPath("open", excludesFile))
			},
			wantErr: errInjected,
		},
		{
			name:  "unreadable filters file",
			files: map[string]string{filtersFile: "./apps\n"},
			fsys: func(base vfs.FS) vfs.FS {
				return newFaultFS(base, atPath("open", filtersFile))
			},
			wantErr: errInjected,
		},
		{
			name:  "malformed filters file",
			files: map[string]string{filtersFile: "[main...\n"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			v := venv.MemVenv()
			for path, content := range tc.files {
				require.NoError(t, vfs.WriteFile(v.FS, path, []byte(content), 0o644))
			}

			if tc.fsys != nil {
				v = v.WithFS(tc.fsys(v.FS))
			}

			actionRan := false

			opts := newWrapTestOptions()
			opts.WorkingDir = testWorkDir

			app, _ := newWrapTestApp()
			cliCtx := newCommandContext(app, nil, &clihelper.Command{Name: "find"})

			err := runWithTelemetry(t, logger.CreateLogger(), opts, v, cliCtx, func(context.Context, *clihelper.Context) error {
				actionRan = true

				return nil
			})
			require.Error(t, err)
			assert.False(t, actionRan, "the action must not run after a setup failure")

			if tc.wantErr != nil {
				require.ErrorIs(t, err, tc.wantErr)
			}
		})
	}
}

// TestWrapWithTelemetryVersionAndParallelism pins the parsed Terragrunt version and the parallelism forced by interactive runs.
func TestWrapWithTelemetryVersionAndParallelism(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name            string
		appVersion      string
		wantVersion     string
		autoApprove     bool
		wantParallelism int
	}{
		{name: "release version", appVersion: "v0.99.1", wantVersion: "0.99.1", autoApprove: true, wantParallelism: 7},
		{
			name:            "malformed version falls back to zero",
			appVersion:      "latest",
			wantVersion:     "0.0.0",
			autoApprove:     true,
			wantParallelism: 7,
		},
		{name: "interactive run forces parallelism one", appVersion: "v0.99.1", wantVersion: "0.99.1", wantParallelism: 1},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			opts := newWrapTestOptions()
			opts.WorkingDir = testWorkDir
			opts.Parallelism = 7
			opts.RunAllAutoApprove = tc.autoApprove
			opts.LogShowAbsPaths = true

			app, _ := newWrapTestApp()
			app.Version = tc.appVersion
			cliCtx := newCommandContext(app, nil, &clihelper.Command{Name: "find"})

			require.NoError(t, runWithTelemetry(t, logger.CreateLogger(), opts, venv.MemVenv(), cliCtx, nil))
			require.NotNil(t, opts.TerragruntVersion)
			assert.Equal(t, tc.wantVersion, opts.TerragruntVersion.String(), "terragrunt version")
			assert.Equal(t, tc.wantParallelism, opts.Parallelism, "parallelism")
		})
	}
}

// TestWrapWithTelemetryReturnsErrors pins that telemetry setup and action failures reach the caller.
func TestWrapWithTelemetryReturnsErrors(t *testing.T) {
	t.Parallel()

	t.Run("telemetry setup", func(t *testing.T) {
		t.Parallel()

		opts := newWrapTestOptions()
		opts.WorkingDir = testWorkDir
		opts.Telemetry.TraceExporter = "http"

		app, _ := newWrapTestApp()
		cliCtx := newCommandContext(app, nil, &clihelper.Command{Name: "find"})

		err := runWithTelemetry(t, logger.CreateLogger(), opts, venv.MemVenv(), cliCtx, nil)

		var missingErr *telemetry.ErrorMissingEnvVariable
		require.ErrorAs(t, err, &missingErr)
	})

	t.Run("action", func(t *testing.T) {
		t.Parallel()

		opts := newWrapTestOptions()
		opts.WorkingDir = testWorkDir

		app, _ := newWrapTestApp()
		cliCtx := newCommandContext(app, nil, &clihelper.Command{Name: "find"})

		err := runWithTelemetry(
			t,
			logger.CreateLogger(),
			opts,
			venv.MemVenv(),
			cliCtx,
			func(context.Context, *clihelper.Context) error {
				return errActionFailed
			},
		)
		require.ErrorIs(t, err, errActionFailed)
	})
}

// TestWrapWithTelemetryRecordsTofuSpanAttributes pins that only commands passing args to OpenTofu record them on the command span.
func TestWrapWithTelemetryRecordsTofuSpanAttributes(t *testing.T) {
	t.Parallel()

	runCmd := &clihelper.Command{Name: run.CommandName}
	shortcutCategory := &clihelper.Category{Name: commands.ShortcutsCommandsCategoryName}

	testCases := []struct {
		name      string
		wantSpan  string
		cmds      []*clihelper.Command
		args      clihelper.Args
		wantAttrs bool
	}{
		{
			name:      "run",
			cmds:      []*clihelper.Command{runCmd},
			args:      clihelper.Args{"plan"},
			wantSpan:  run.CommandName,
			wantAttrs: true,
		},
		{
			name:      "shortcut",
			cmds:      []*clihelper.Command{{Name: "plan", Category: shortcutCategory}},
			wantSpan:  "plan",
			wantAttrs: true,
		},
		{
			name:      "run subcommand",
			cmds:      []*clihelper.Command{runCmd, {Name: "plan"}},
			wantSpan:  "plan",
			wantAttrs: true,
		},
		{
			name:     "run without a command",
			cmds:     []*clihelper.Command{runCmd},
			wantSpan: run.CommandName,
		},
		{
			name:     "discovery command",
			cmds:     []*clihelper.Command{{Name: "find"}},
			wantSpan: "find",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			opts := newWrapTestOptions()
			opts.WorkingDir = testWorkDir
			opts.Telemetry.TraceExporter = "console"

			app, out := newWrapTestApp()
			cliCtx := newCommandContext(app, tc.args, tc.cmds...)

			require.NoError(t, runWithTelemetry(t, logger.CreateLogger(), opts, venv.MemVenv(), cliCtx, nil))

			attrs, found := spanAttributes(t, out, tc.wantSpan)
			require.True(t, found, "span %s should be exported", tc.wantSpan)

			_, hasCommand := attrs["tofu_command"]
			_, hasArgs := attrs["args"]

			assert.Equal(t, tc.wantAttrs, hasCommand, "tofu_command attribute presence")
			assert.Equal(t, tc.wantAttrs, hasArgs, "args attribute presence")

			if tc.wantAttrs {
				assert.Equal(t, opts.TofuCommand, attrs["tofu_command"], "tofu_command should be the resolved command")
			}
		})
	}
}

// newWrapTestOptions returns options that skip the auto provider cache probe, which has its own tests.
func newWrapTestOptions() *options.TerragruntOptions {
	opts := options.NewTerragruntOptions(vexec.NewOSExec())
	opts.NoAutoProviderCacheDir = true

	return opts
}

// newWrapTestApp returns an app whose stdout is captured, for exporters that write there.
func newWrapTestApp() (*clihelper.App, *bytes.Buffer) {
	out := new(bytes.Buffer)

	app := clihelper.NewApp(map[string]string{})
	app.Name = "terragrunt"
	app.Version = "v0.99.1"
	app.Writer = out
	app.ErrWriter = io.Discard

	return app, out
}

// newCommandContext nests a context for each command under the app context, all sharing args.
func newCommandContext(app *clihelper.App, args clihelper.Args, cmds ...*clihelper.Command) *clihelper.Context {
	cliCtx := clihelper.NewAppContext(app, args)

	for _, cmd := range cmds {
		cliCtx = cliCtx.NewCommandContext(cmd, args)
	}

	return cliCtx
}

// runWithTelemetry runs action, or a no-op, through WrapWithTelemetry.
func runWithTelemetry(
	t *testing.T,
	l log.Logger,
	opts *options.TerragruntOptions,
	v *venv.Venv,
	cliCtx *clihelper.Context,
	action clihelper.ActionFunc,
) error {
	t.Helper()

	if action == nil {
		action = func(context.Context, *clihelper.Context) error { return nil }
	}

	return commands.WrapWithTelemetry(l, opts, v)(t.Context(), cliCtx, action)
}

// mustParseFilters parses queries into filters, failing the test on a parse error.
func mustParseFilters(t *testing.T, queries []string) filter.Filters {
	t.Helper()

	filters, err := filter.ParseFilterQueries(logger.CreateLogger(), queries)
	require.NoError(t, err)

	return filters
}

// filterQueries returns the original query of each filter, in order.
func filterQueries(filters filter.Filters) []string {
	queries := make([]string, 0, len(filters))

	for _, f := range filters {
		queries = append(queries, f.String())
	}

	return queries
}

// spanAttributes decodes the console exporter output and returns the string attributes of the named span.
func spanAttributes(t *testing.T, out *bytes.Buffer, name string) (map[string]any, bool) {
	t.Helper()

	type exportedSpan struct {
		Name       string `json:"Name"`
		Attributes []struct {
			Value struct {
				Value any `json:"Value"`
			} `json:"Value"`
			Key string `json:"Key"`
		} `json:"Attributes"`
	}

	dec := json.NewDecoder(out)

	for dec.More() {
		var span exportedSpan
		require.NoError(t, dec.Decode(&span))

		if span.Name != name {
			continue
		}

		attrs := make(map[string]any, len(span.Attributes))
		for _, attr := range span.Attributes {
			attrs[attr.Key] = attr.Value.Value
		}

		return attrs, true
	}

	return nil, false
}
