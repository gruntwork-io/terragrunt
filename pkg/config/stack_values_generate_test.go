package config_test

import (
	"fmt"
	"maps"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hashicorp/hcl/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/internal/worker"
	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"
)

var valuesGenerateRoot = venvtest.Root("/fixture")

// valuesStackPath is the stack file every test here generates.
var valuesStackPath = filepath.Join(valuesGenerateRoot, "live", config.DefaultStackFile)

// valuesCatalog is the catalog every test here generates from, keyed by paths under valuesGenerateRoot.
var valuesCatalog = map[string]string{
	"catalog/units/app/terragrunt.hcl":       unitValuesBlock,
	"catalog/units/app/main.tf":              "",
	"catalog/units/json/main.tf":             "",
	"catalog/units/json/terragrunt.hcl.json": `{"values": {"value": {"instance_count": {"type": "number"}}}}`,
	"catalog/stacks/service/terragrunt.stack.hcl": `values {
  value "env" {
    type = string
  }
}

unit "app" {
  source = "../../units/app"
  path   = "app"

  values = {
    instance_count = 1
  }
}
`,
}

// valuesGeneration is a stack generate run over valuesCatalog and more files.
type valuesGeneration struct {
	files              map[string]string
	sourceMap          map[string]string
	diagnosticsHandler func(*hcl.File, hcl.Diagnostics) (hcl.Diagnostics, error)
	workers            int
	experiment         valuesExperiment
}

func TestGenerateStackFileRejectsUndeclaredValue(t *testing.T) {
	t.Parallel()

	v, err := valuesGeneration{
		files: map[string]string{
			"live/terragrunt.stack.hcl": `unit "app" {
  source = "../catalog/units/app"
  path   = "app"

  values = {
    instance_count = 2
    tag            = { team = "core" }
  }
}
`,
		},
		experiment: valuesExperimentOn,
	}.run(t)

	diag := singleDiagnostic(t, err)
	assert.Equal(t, `Undeclared value "tag" for unit "app"`, diag.Summary)
	assert.Contains(t, diag.Detail, `Did you mean "tags"?`)
	assert.Equal(t, valuesStackPath, diag.Subject.Filename)
	assert.Equal(t, 7, diag.Subject.Start.Line)
	assert.False(t, vfs.IsFile(v.FS, generatedValuesFile("app")))
}

func TestGenerateStackFileReportsValuesErrorAtInclude(t *testing.T) {
	t.Parallel()

	_, err := valuesGeneration{
		files: map[string]string{
			"live/terragrunt.stack.hcl": `include "units" {
  path = "units.hcl"
}
`,
			"live/units.hcl": `unit "app" {
  source = "../catalog/units/app"
  path   = "app"

  values = {
    instance_count = 2
    tag            = "core"
  }
}
`,
		},
		experiment: valuesExperimentOn,
	}.run(t)

	diag := singleDiagnostic(t, err)
	assert.Equal(t, `Undeclared value "tag" for unit "app"`, diag.Summary)
	assert.Equal(t, filepath.Join(valuesGenerateRoot, "live", "units.hcl"), diag.Subject.Filename)
	assert.Equal(t, 7, diag.Subject.Start.Line)
}

func TestGenerateStackFileReportsValuesErrorAtStackAutoinclude(t *testing.T) {
	t.Parallel()

	_, err := valuesGeneration{
		files: map[string]string{
			"live/terragrunt.stack.hcl": `unit "app" {
  source = "../catalog/units/app"
  path   = "app"

  values = {
    instance_count = 2
  }
}
`,
			"live/" + config.DefaultAutoIncludeStackFile: `unit "app" {
  source = "../catalog/units/app"
  path   = "app"

  values = {
    instance_count = 2
    tag            = "core"
  }
}
`,
		},
		experiment: valuesExperimentOn,
	}.run(t)

	diag := singleDiagnostic(t, err)
	assert.Equal(t, `Undeclared value "tag" for unit "app"`, diag.Summary)
	assert.Equal(t, filepath.Join(valuesGenerateRoot, "live", config.DefaultAutoIncludeStackFile), diag.Subject.Filename)
	assert.Equal(t, 7, diag.Subject.Start.Line)
}

// TestGenerateStackFileReportsValuesErrorAtExpandedBlockSharingALabel pins that the values of an expanded block are
// checked against its own values attribute when an earlier block with the same label sets no values.
func TestGenerateStackFileReportsValuesErrorAtExpandedBlockSharingALabel(t *testing.T) {
	t.Parallel()

	_, err := valuesGeneration{
		files: map[string]string{
			"catalog/units/optional/terragrunt.hcl": "values {\n  value \"n\" {\n    default = 1\n  }\n}\n",
			"catalog/units/optional/main.tf":        "",
			"live/terragrunt.stack.hcl": `unit "app" {
  expansion {
    for_each = toset(["a"])
  }

  source = "../catalog/units/optional"
  path   = "optional/${each.key}"
}

unit "app" {
  expansion {
    for_each = toset(["b"])
  }

  source = "../catalog/units/app"
  path   = "app/${each.key}"

  values = {
    instance_count = 1
    tag            = each.key
  }
}
`,
		},
		experiment: valuesExperimentOn,
	}.run(t)

	diag := singleDiagnostic(t, err)
	assert.Equal(t, `Undeclared value "tag" for unit "app[b]"`, diag.Summary)
	assert.Equal(t, valuesStackPath, diag.Subject.Filename)
	assert.Equal(t, 20, diag.Subject.Start.Line)
}

func TestGenerateStackFileReportsMissingValueAtBlock(t *testing.T) {
	t.Parallel()

	_, err := valuesGeneration{
		files: map[string]string{
			"live/terragrunt.stack.hcl": `locals {
  env = "dev"
}

unit "app" {
  source = "../catalog/units/app"
  path   = "app"
}
`,
		},
		experiment: valuesExperimentOn,
	}.run(t)

	diag := singleDiagnostic(t, err)
	assert.Equal(t, `Missing required value "instance_count" for unit "app"`, diag.Summary)
	assert.Equal(t, valuesStackPath, diag.Subject.Filename)
	assert.Equal(t, 5, diag.Subject.Start.Line)
	assert.Equal(t, 1, diag.Subject.Start.Column)
}

func TestGenerateStackFileNamesExpandedInstance(t *testing.T) {
	t.Parallel()

	_, err := valuesGeneration{
		files: map[string]string{
			"live/terragrunt.stack.hcl": `unit "app" {
  expansion {
    for_each = toset(["a"])
  }

  source = "../catalog/units/app"
  path   = "app/${each.key}"

  values = {
    instance_count = 1
    tag            = each.key
  }
}
`,
		},
		experiment: valuesExperimentOn,
	}.run(t)

	diag := singleDiagnostic(t, err)
	assert.Equal(t, `Undeclared value "tag" for unit "app[a]"`, diag.Summary)
	assert.Equal(t, 11, diag.Subject.Start.Line)
}

func TestGenerateStackFileReportsMistypedValueAtExpression(t *testing.T) {
	t.Parallel()

	_, err := valuesGeneration{
		files: map[string]string{
			"live/terragrunt.stack.hcl": `unit "app" {
  source = "../catalog/units/app"
  path   = "app"

  values = {
    instance_count = "many"
  }
}
`,
		},
		experiment: valuesExperimentOn,
	}.run(t)

	diag := singleDiagnostic(t, err)
	assert.Equal(t, `Invalid value "instance_count" for unit "app"`, diag.Summary)
	assert.Equal(t, 6, diag.Subject.Start.Line)
	assert.Equal(t, 22, diag.Subject.Start.Column)
}

func TestGenerateStackFileWritesSuppliedValuesUnresolved(t *testing.T) {
	t.Parallel()

	v, err := valuesGeneration{
		files: map[string]string{
			"live/terragrunt.stack.hcl": `unit "app" {
  source = "../catalog/units/app"
  path   = "app"

  values = {
    instance_count = "2"
  }
}
`,
		},
		experiment: valuesExperimentOn,
	}.run(t)
	require.NoError(t, err)

	written, err := vfs.ReadFile(v.FS, generatedValuesFile("app"))
	require.NoError(t, err)
	assert.Equal(
		t,
		"# Auto-generated by the terragrunt.stack.hcl file by Terragrunt. Do not edit manually\n"+
			`instance_count = "2"`+"\n",
		string(written),
	)
}

func TestGenerateStackFileSkipsUnknownValues(t *testing.T) {
	t.Parallel()

	var summaries []string

	_, err := valuesGeneration{
		files: map[string]string{
			"live/terragrunt.stack.hcl": `values {
  value "size" {
    type = number
  }
}

unit "app" {
  source = "../catalog/units/app"
  path   = "app"

  values = {
    instance_count = values.size
    tag            = "core"
  }
}
`,
			"live/terragrunt.values.hcl": `size = "many"`,
		},
		diagnosticsHandler: func(_ *hcl.File, diags hcl.Diagnostics) (hcl.Diagnostics, error) {
			for _, diag := range diags {
				summaries = append(summaries, diag.Summary)
			}

			return nil, nil
		},
		experiment: valuesExperimentOn,
	}.run(t)
	require.NoError(t, err)

	assert.Equal(t, []string{`Invalid value "size"`}, summaries)
}

func TestGenerateStackFileLeavesShippedValuesFileToTheUnit(t *testing.T) {
	t.Parallel()

	v, err := valuesGeneration{
		files: map[string]string{
			"catalog/units/app/terragrunt.values.hcl": `instance_count = 2`,
			"live/terragrunt.stack.hcl": `unit "app" {
  source = "../catalog/units/app"
  path   = "app"
}
`,
		},
		experiment: valuesExperimentOn,
	}.run(t)
	require.NoError(t, err)

	written, err := vfs.ReadFile(v.FS, generatedValuesFile("app"))
	require.NoError(t, err)
	assert.Equal(t, `instance_count = 2`, string(written))
}

func TestGenerateStackFileChecksNestedStackValues(t *testing.T) {
	t.Parallel()

	v, err := valuesGeneration{
		files: map[string]string{
			"live/terragrunt.stack.hcl": `stack "service" {
  source = "../catalog/stacks/service"
  path   = "service"

  values = {
    evn = "prod"
  }
}
`,
		},
		experiment: valuesExperimentOn,
	}.run(t)

	var diags hcl.Diagnostics

	require.ErrorAs(t, err, &diags)
	require.Len(t, diags, 2)
	assert.Equal(t, `Undeclared value "evn" for stack "service"`, diags[0].Summary)
	assert.Contains(t, diags[0].Detail, `Did you mean "env"?`)
	assert.Equal(t, `Missing required value "env" for stack "service"`, diags[1].Summary)
	assert.False(t, vfs.IsFile(v.FS, generatedValuesFile("service")))
}

func TestGenerateStackFileChecksJSONUnitValues(t *testing.T) {
	t.Parallel()

	_, err := valuesGeneration{
		files: map[string]string{
			"live/terragrunt.stack.hcl": `unit "app" {
  source        = "../catalog/units/json"
  path          = "app"
  no_validation = true

  values = {
    instance_count = "many"
  }
}
`,
		},
		experiment: valuesExperimentOn,
	}.run(t)

	diag := singleDiagnostic(t, err)
	assert.Equal(t, `Invalid value "instance_count" for unit "app"`, diag.Summary)
}

func TestGenerateStackFileRedactsSourceInValuesError(t *testing.T) {
	t.Parallel()

	_, err := valuesGeneration{
		files: map[string]string{
			"live/terragrunt.stack.hcl": `unit "app" {
  source = "git::https://fake-user:fake-token@example.com/org/catalog.git//units/app?ref=v1.0.0"
  path   = "app"

  values = {
    instance_count = 2
    tag            = "core"
  }
}
`,
		},
		sourceMap: map[string]string{
			"git::https://fake-user:fake-token@example.com/org/catalog.git": filepath.Join(valuesGenerateRoot, "catalog"),
		},
		experiment: valuesExperimentOn,
	}.run(t)

	diag := singleDiagnostic(t, err)
	assert.Contains(t, diag.Detail, "example.com/org/catalog.git//units/app?ref=v1.0.0")
	assert.NotContains(t, diag.Detail, "fake-token")
}

func TestGenerateStackFileWithoutExperimentWritesValuesUnchecked(t *testing.T) {
	t.Parallel()

	v, err := valuesGeneration{
		files: map[string]string{
			"live/terragrunt.stack.hcl": `unit "app" {
  source = "../catalog/units/app"
  path   = "app"

  values = {
    tag = "core"
  }
}
`,
		},
		experiment: valuesExperimentOff,
	}.run(t)
	require.NoError(t, err)

	written, err := vfs.ReadFile(v.FS, generatedValuesFile("app"))
	require.NoError(t, err)
	assert.Contains(t, string(written), `tag = "core"`)
}

func TestGenerateStackFileReportsEachUnitValuesErrorOnceWithRacing(t *testing.T) {
	t.Parallel()

	const units = 16

	var stack strings.Builder

	for i := range units {
		fmt.Fprintf(&stack, `
unit "app_%d" {
  source = "../catalog/units/app"
  path   = "app_%d"

  values = {
    instance_count = 1
    tag            = "core"
  }
}
`, i, i)
	}

	counts := map[string]int{}

	_, err := valuesGeneration{
		files: map[string]string{"live/terragrunt.stack.hcl": stack.String()},
		diagnosticsHandler: func(_ *hcl.File, diags hcl.Diagnostics) (hcl.Diagnostics, error) {
			for _, diag := range diags {
				counts[diag.Summary]++
			}

			return diags, nil
		},
		workers:    8,
		experiment: valuesExperimentOn,
	}.run(t)
	require.Error(t, err)

	want := make(map[string]int, units)

	for i := range units {
		want[fmt.Sprintf(`Undeclared value "tag" for unit "app_%d"`, i)] = 1
	}

	assert.Equal(t, want, counts)
}

// run generates valuesStackPath from valuesCatalog and g.files, and returns the venv and the error of the generation.
func (g valuesGeneration) run(t *testing.T) (*venv.Venv, error) {
	t.Helper()

	files := maps.Clone(valuesCatalog)
	maps.Copy(files, g.files)

	v := writeValuesFixture(t, valuesGenerateRoot, files)

	ctx, pctx := newValuesTestContext(t, valuesStackPath, g.experiment)
	pctx.TerragruntStackConfigPath = valuesStackPath
	pctx.NoCAS = true

	if g.sourceMap != nil {
		pctx.SourceMap = g.sourceMap
	}

	if g.diagnosticsHandler != nil {
		settings := pctx.Parser
		settings.DiagnosticsHandler = g.diagnosticsHandler
		pctx = pctx.WithParserSettings(settings)
	}

	pool := worker.NewWorkerPool(max(1, g.workers))
	pool.Start()

	defer pool.Stop()

	err := config.GenerateStackFile(ctx, logger.CreateLogger(), v, new(config.Store), pctx, pool, valuesStackPath)
	if err != nil {
		return v, err
	}

	return v, pool.Wait()
}

// singleDiagnostic returns the one diagnostic err holds.
func singleDiagnostic(t *testing.T, err error) *hcl.Diagnostic {
	t.Helper()

	var diags hcl.Diagnostics

	require.ErrorAs(t, err, &diags)
	require.Len(t, diags, 1)

	return diags[0]
}

// generatedValuesFile returns the path of the values file generated for the component at path in valuesStackPath.
func generatedValuesFile(path string) string {
	return filepath.Join(filepath.Dir(valuesStackPath), config.StackDir, path, "terragrunt.values.hcl")
}
