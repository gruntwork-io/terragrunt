package config_test

import (
	"bytes"
	"testing"

	"github.com/hashicorp/hcl/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gruntwork-io/terragrunt/internal/strict/controls"
	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/pkg/config/hclparse"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"
)

const (
	errorOutsideCatalog = `other = local.missing`
	errorInsideCatalog  = `
catalog {
  urls = [local.missing]
}
`
	bareInclude = `
include {
  path = "root.hcl"
}
`
)

type catalogOrOther struct {
	Catalog *struct {
		URLs []string `hcl:"urls,attr"`
	} `hcl:"catalog,block"`
	Other string `hcl:"other,optional"`
}

type labeledInclude struct {
	Include []struct {
		Name string `hcl:"name,label"`
		Path string `hcl:"path,attr"`
	} `hcl:"include,block"`
}

func TestParserOptions(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name          string
		content       string
		settings      config.ParserSettings
		level         log.Level
		recordHandler bool
		wantErr       bool
		wantErrWriter bool
		wantHandled   bool
	}{
		{
			name:     "logged diagnostics skip the error writer",
			content:  errorOutsideCatalog,
			settings: config.ParserSettings{Diagnostics: config.DiagnosticsLogged},
			level:    log.DebugLevel,
			wantErr:  true,
		},
		{
			name:          "suppressed diagnostics reach the error writer at debug level",
			content:       errorOutsideCatalog,
			settings:      config.ParserSettings{Diagnostics: config.DiagnosticsSuppressed},
			level:         log.DebugLevel,
			wantErr:       true,
			wantErrWriter: true,
		},
		{
			name:     "suppressed diagnostics are discarded above debug level",
			content:  errorOutsideCatalog,
			settings: config.ParserSettings{Diagnostics: config.DiagnosticsSuppressed},
			level:    log.InfoLevel,
			wantErr:  true,
		},
		{
			name:     "discarded diagnostics skip the error writer at debug level",
			content:  errorOutsideCatalog,
			settings: config.ParserSettings{Diagnostics: config.DiagnosticsDiscarded},
			level:    log.DebugLevel,
			wantErr:  true,
		},
		{
			name:    "skipped defaults keep the suppressed writer",
			content: errorOutsideCatalog,
			settings: config.ParserSettings{
				Diagnostics:  config.DiagnosticsSuppressed,
				SkipDefaults: true,
			},
			level:         log.DebugLevel,
			wantErr:       true,
			wantErrWriter: true,
		},
		{
			name:          "handler sees diagnostics",
			content:       errorOutsideCatalog,
			settings:      config.ParserSettings{Diagnostics: config.DiagnosticsDiscarded},
			level:         log.DebugLevel,
			recordHandler: true,
			wantErr:       true,
			wantHandled:   true,
		},
		{
			name:    "ignored diagnostics run before the handler",
			content: errorOutsideCatalog,
			settings: config.ParserSettings{
				Diagnostics:       config.DiagnosticsSuppressed,
				IgnoreDiagnostics: true,
			},
			level:         log.DebugLevel,
			recordHandler: true,
		},
		{
			name:    "halt on error drops errors outside the listed blocks before the handler",
			content: errorOutsideCatalog,
			settings: config.ParserSettings{
				Diagnostics:             config.DiagnosticsSuppressed,
				HaltOnErrorOnlyInBlocks: []string{config.MetadataCatalog},
			},
			level:         log.DebugLevel,
			recordHandler: true,
		},
		{
			name:    "halt on error keeps errors inside the listed blocks for the handler",
			content: errorInsideCatalog,
			settings: config.ParserSettings{
				Diagnostics:             config.DiagnosticsSuppressed,
				HaltOnErrorOnlyInBlocks: []string{config.MetadataCatalog},
			},
			level:         log.DebugLevel,
			recordHandler: true,
			wantErr:       true,
			wantErrWriter: true,
			wantHandled:   true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var errBuf bytes.Buffer

			v := venvtest.New().WithErrWriter(&errBuf)
			l := logger.CreateLogger().WithOptions(log.WithLevel(tc.level))

			var handled hcl.Diagnostics

			settings := tc.settings
			if tc.recordHandler {
				settings.DiagnosticsHandler = func(_ *hcl.File, diags hcl.Diagnostics) (hcl.Diagnostics, error) {
					handled = append(handled, diags...)
					return diags, nil
				}
			}

			file, err := hclparse.NewParser(config.ParserOptions(l, v, settings)...).
				ParseFromString(tc.content, "terragrunt.hcl")
			require.NoError(t, err)

			err = file.Decode(&catalogOrOther{}, nil)
			if tc.wantErr {
				require.Error(t, err)
			}

			if !tc.wantErr {
				require.NoError(t, err)
			}

			assert.Equal(t, tc.wantErrWriter, errBuf.Len() > 0)
			assert.Equal(t, tc.wantHandled, handled.HasErrors())
		})
	}
}

func TestParserOptionsBareIncludeRewrite(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		settings config.ParserSettings
		wantErr  bool
	}{
		{
			name:     "rewrite labels a bare include",
			settings: config.ParserSettings{RewriteBareInclude: true, Diagnostics: config.DiagnosticsDiscarded},
		},
		{
			name:     "no rewrite leaves a bare include unlabeled",
			settings: config.ParserSettings{Diagnostics: config.DiagnosticsDiscarded},
			wantErr:  true,
		},
		{
			name: "skipped defaults leave out the rewrite",
			settings: config.ParserSettings{
				RewriteBareInclude: true,
				SkipDefaults:       true,
				Diagnostics:        config.DiagnosticsDiscarded,
			},
			wantErr: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			l := logger.CreateLogger()

			file, err := hclparse.NewParser(config.ParserOptions(l, venvtest.New(), tc.settings)...).
				ParseFromString(bareInclude, "terragrunt.hcl")
			require.NoError(t, err)

			var out labeledInclude

			err = file.Decode(&out, nil)
			if tc.wantErr {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			require.Len(t, out.Include, 1)
			assert.Empty(t, out.Include[0].Name)
			assert.Equal(t, "root.hcl", out.Include[0].Path)
		})
	}
}

func TestDefaultParserSettingsFollowsBareIncludeControl(t *testing.T) {
	t.Parallel()

	assert.True(t, config.DefaultParserSettings(t.Context(), controls.New()).RewriteBareInclude)

	strictControls := controls.New()
	require.NoError(t, strictControls.EnableControl(controls.BareInclude))

	assert.False(t, config.DefaultParserSettings(t.Context(), strictControls).RewriteBareInclude)
}

func TestCloneCopiesHaltOnErrorBlocks(t *testing.T) {
	t.Parallel()

	_, pctx := config.NewParsingContext(t.Context(), logger.CreateLogger(), venvtest.New())
	pctx.Parser.HaltOnErrorOnlyInBlocks = []string{config.MetadataCatalog}

	clone := pctx.Clone()
	clone.Parser.HaltOnErrorOnlyInBlocks[0] = config.MetadataInclude

	assert.Equal(t, []string{config.MetadataCatalog}, pctx.Parser.HaltOnErrorOnlyInBlocks)
}
