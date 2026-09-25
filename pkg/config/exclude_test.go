package config_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestPartialParseExcludeWithWrongTypeErrors pins that an exclude block
// whose attributes do not decode fails the parse, where it used to vanish
// from the parsed config.
func TestPartialParseExcludeWithWrongTypeErrors(t *testing.T) {
	t.Parallel()

	cfgPath := writeExcludeUnit(t, `
exclude {
  if      = true
  actions = "plan"
}
`)

	ctx, pctx := newTestParsingContext(t, venvtest.NewWithOSFS(), cfgPath)
	pctx = pctx.WithDecodeList(config.ExcludeBlock)

	_, err := config.PartialParseConfigFile(ctx, pctx, logger.CreateLogger(), cfgPath, nil)
	require.ErrorAs(t, err, new(config.InvalidExcludeBlockError))
}

// TestPartialParseExcludeReadingDependencyOutputDuringDiscovery pins that
// discovery, which parses without dependency outputs, skips an exclude block
// that reads one and still parses the rest of the config.
func TestPartialParseExcludeReadingDependencyOutputDuringDiscovery(t *testing.T) {
	t.Parallel()

	cfgPath := writeExcludeUnit(t, `
dependency "dep" {
  config_path  = "../dep"
  mock_outputs = { flag = true }
}

exclude {
  if      = dependency.dep.outputs.flag
  actions = ["plan"]
}
`)

	ctx, pctx := newTestParsingContext(t, venvtest.NewWithOSFS(), cfgPath)
	pctx = pctx.WithDecodeList(config.DependencyBlock, config.ExcludeBlock)
	pctx.SkipOutputsResolution = true

	parsed, err := config.PartialParseConfigFile(ctx, pctx, logger.CreateLogger(), cfgPath, nil)
	require.NoError(t, err)
	assert.Nil(t, parsed.Exclude)
	assert.Len(t, parsed.TerragruntDependencies, 1)
}

// TestPartialParseExcludeWithNullUnknownOrSensitiveString pins that discovery parses null, unknown and sensitive string flags like bools instead of panicking.
func TestPartialParseExcludeWithNullUnknownOrSensitiveString(t *testing.T) {
	t.Parallel()

	tests := []struct {
		want      *config.ExcludeConfig
		wantErrAs any
		name      string
		exclude   string
		wantWarn  bool
	}{
		{
			name:    "null bool if",
			exclude: `if = null`,
			want:    &config.ExcludeConfig{Actions: []string{"plan"}},
		},
		{
			name:    "null string if",
			exclude: `if = tostring(null)`,
			want:    &config.ExcludeConfig{Actions: []string{"plan"}},
		},
		{
			name:    "null string no_run",
			exclude: "if = true\nno_run = tostring(null)",
			want:    &config.ExcludeConfig{If: true, Actions: []string{"plan"}},
		},
		{
			name:    "null string exclude_dependencies",
			exclude: "if = true\nexclude_dependencies = tostring(null)",
			want:    &config.ExcludeConfig{If: true, Actions: []string{"plan"}},
		},
		{
			name:     "unknown dependency output if",
			exclude:  `if = dependency.dep.outputs.flag`,
			wantWarn: true,
		},
		{
			name:     "unknown string if",
			exclude:  `if = tostring(dependency.dep.outputs.flag)`,
			wantWarn: true,
		},
		{
			name:     "unknown interpolated string if",
			exclude:  `if = "${dependency.dep.outputs.flag}"`,
			wantWarn: true,
		},
		{
			name:     "unknown string no_run",
			exclude:  "if = true\nno_run = tostring(dependency.dep.outputs.flag)",
			wantWarn: true,
		},
		{
			name:     "unknown string exclude_dependencies",
			exclude:  "if = true\nexclude_dependencies = tostring(dependency.dep.outputs.flag)",
			wantWarn: true,
		},
		{
			name:      "sensitive bool if",
			exclude:   `if = sensitive(true)`,
			wantErrAs: new(config.InvalidExcludeBlockError),
		},
		{
			name:      "sensitive string if",
			exclude:   `if = sensitive("true")`,
			wantErrAs: new(config.InvalidExcludeBlockError),
		},
		{
			name:      "sensitive null string if",
			exclude:   `if = sensitive(tostring(null))`,
			wantErrAs: new(config.InvalidExcludeBlockError),
		},
		{
			name:     "sensitive unknown string if",
			exclude:  `if = sensitive(tostring(dependency.dep.outputs.flag))`,
			wantWarn: true,
		},
		{
			name:    "known string flags",
			exclude: "if = \"true\"\nno_run = \"false\"\nexclude_dependencies = \"true\"",
			want: &config.ExcludeConfig{
				If:                  true,
				Actions:             []string{"plan"},
				NoRun:               new(false),
				ExcludeDependencies: new(true),
			},
		},
		{
			name:      "invalid string if",
			exclude:   `if = "maybe"`,
			wantErrAs: new(*strconv.NumError),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			root := venvtest.Root("/live")
			unitPath := filepath.Join("unit", config.DefaultTerragruntConfigPath)
			cfgPath := filepath.Join(root, unitPath)
			fsys := venvtest.NewFS(t, root, map[string]string{
				filepath.Join("dep", config.DefaultTerragruntConfigPath): "",
				unitPath: `
dependency "dep" {
  config_path = "../dep"
}

exclude {
  actions = ["plan"]
  ` + tt.exclude + `
}
`,
			})

			var logBuf bytes.Buffer

			l := logger.CreateLogger()
			l.SetOptions(log.WithOutput(&logBuf))

			ctx, pctx := newTestParsingContext(t, venvtest.New().WithFS(fsys), cfgPath)
			pctx = pctx.WithDecodeList(config.DependencyBlock, config.ExcludeBlock).WithSkipOutputsResolution()

			parsed, err := config.PartialParseConfigFile(ctx, pctx, l, cfgPath, nil)
			if tt.wantErrAs != nil {
				require.ErrorAs(t, err, tt.wantErrAs)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.want, parsed.Exclude)
			assert.Equal(
				t,
				tt.wantWarn,
				strings.Contains(logBuf.String(), "Ignoring the exclude block in "+cfgPath),
				logBuf.String(),
			)
		})
	}
}

// TestParseConfigFileExcludeInIncludeWithNullString pins that an included exclude block with a null string if errors instead of panicking.
func TestParseConfigFileExcludeInIncludeWithNullString(t *testing.T) {
	t.Parallel()

	root := venvtest.Root("/live")
	cfgPath := filepath.Join(root, "app", config.DefaultTerragruntConfigPath)
	fsys := venvtest.NewFS(t, root, map[string]string{
		"root.hcl": `
exclude {
  if      = tostring(null)
  actions = ["all"]
}
`,
		filepath.Join("app", config.DefaultTerragruntConfigPath): `
include "root" {
  path = find_in_parent_folders("root.hcl")
}
`,
	})

	ctx, pctx := newTestParsingContext(t, venvtest.New().WithFS(fsys), cfgPath)

	_, err := config.ParseConfigFile(ctx, pctx, logger.CreateLogger(), cfgPath, nil)
	require.ErrorContains(t, err, "null value is not allowed")
	assert.ErrorContains(t, err, filepath.Join(root, "root.hcl"))
}

// writeExcludeUnit writes body as a unit config beside an empty dependency
// unit named dep, and returns the unit's config path.
func writeExcludeUnit(t *testing.T, body string) string {
	t.Helper()

	tmpDir := t.TempDir()

	depDir := filepath.Join(tmpDir, "dep")
	require.NoError(t, os.MkdirAll(depDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(depDir, config.DefaultTerragruntConfigPath), nil, 0644))

	unitDir := filepath.Join(tmpDir, "unit")
	require.NoError(t, os.MkdirAll(unitDir, 0755))

	cfgPath := filepath.Join(unitDir, config.DefaultTerragruntConfigPath)
	require.NoError(t, os.WriteFile(cfgPath, []byte(body), 0644))

	return cfgPath
}

func TestExcludeConfig_ShouldPreventRun(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		action  string
		exclude config.ExcludeConfig
		want    bool
	}{
		{
			name: "output in actions with if=true prevents output",
			exclude: config.ExcludeConfig{
				If:      true,
				Actions: []string{"plan", "apply", "destroy", "output"},
			},
			action: "output",
			want:   true,
		},
		{
			name: "output in actions with if=false does not prevent output",
			exclude: config.ExcludeConfig{
				If:      false,
				Actions: []string{"output"},
			},
			action: "output",
			want:   false,
		},
		{
			name: "output not in actions does not prevent output",
			exclude: config.ExcludeConfig{
				If:      true,
				Actions: []string{"plan", "apply"},
			},
			action: "output",
			want:   false,
		},
		{
			name: "all actions prevents output",
			exclude: config.ExcludeConfig{
				If:      true,
				Actions: []string{"all"},
				NoRun:   new(true),
			},
			action: "output",
			want:   true,
		},
		{
			name: "all_except_output does not prevent output",
			exclude: config.ExcludeConfig{
				If:      true,
				Actions: []string{"all_except_output"},
				NoRun:   new(true),
			},
			action: "output",
			want:   false,
		},
		{
			name: "no_run=false never prevents",
			exclude: config.ExcludeConfig{
				If:      true,
				Actions: []string{"output"},
				NoRun:   new(false),
			},
			action: "output",
			want:   false,
		},
		{
			name: "empty actions does not prevent",
			exclude: config.ExcludeConfig{
				If:      true,
				Actions: []string{},
			},
			action: "output",
			want:   false,
		},
		{
			name: "plan action does not prevent output",
			exclude: config.ExcludeConfig{
				If:      true,
				Actions: []string{"plan"},
			},
			action: "output",
			want:   false,
		},
		{
			name: "all actions with no_run nil uses legacy Contains",
			exclude: config.ExcludeConfig{
				If:      true,
				Actions: []string{"all"},
			},
			action: "output",
			want:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := tt.exclude.ShouldPreventRun(tt.action)
			assert.Equal(t, tt.want, got)
		})
	}
}
