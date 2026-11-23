package validate_test

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gruntwork-io/terragrunt/internal/experiment"
	"github.com/gruntwork-io/terragrunt/internal/view/diagnostic"
	"github.com/gruntwork-io/terragrunt/pkg/config"
)

const tagsValuesBlock = `values {
  value "tags" {
    type    = map(string)
    default = {}
  }
}
`

const tryAdvice = "To make a value optional, declare it with a default in the values block, and read it " +
	"without try()."

func TestRunValidateValuesReferences(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name   string
		block  string
		inputs string
		detail string
		line   int
	}{
		{
			name:   "inside try",
			block:  tagsValuesBlock,
			inputs: "inputs = {\n  tags = try(values.tgas, {})\n}\n",
			detail: "The values block of this unit does not declare \"tgas\", so try() always returns its fallback.\n\n" +
				"Did you mean \"tags\"?\n\n" + tryAdvice,
			line: 9,
		},
		{
			name:   "inside can",
			block:  tagsValuesBlock,
			inputs: "inputs = {\n  tagged = can(values.tgas) ? \"yes\" : \"no\"\n}\n",
			detail: "The values block of this unit does not declare \"tgas\", so can() always returns false.\n\n" +
				"Did you mean \"tags\"?\n\n" +
				"To make a value optional, declare it with a default in the values block, and read it without can().",
			line: 9,
		},
		{
			name:   "innermost guard",
			block:  tagsValuesBlock,
			inputs: "inputs = {\n  tagged = try(can(values.tgas) ? 1 : 0, 2)\n}\n",
			detail: "The values block of this unit does not declare \"tgas\", so can() always returns false.\n\n" +
				"Did you mean \"tags\"?\n\n" +
				"To make a value optional, declare it with a default in the values block, and read it without can().",
			line: 9,
		},
		{
			name:   "for expression",
			block:  tagsValuesBlock,
			inputs: "inputs = {\n  names = [for k, v in values.tgas : upper(v)]\n}\n",
			detail: "The values block of this unit does not declare \"tgas\".\n\n" +
				"Did you mean \"tags\"?\n\n" +
				"Fix the reference, or declare \"tgas\" in the values block.",
			line: 9,
		},
		{
			name:   "try inside a for expression",
			block:  tagsValuesBlock,
			inputs: "inputs = {\n  names = [for k, v in try(values.tgas, {}) : k]\n}\n",
			detail: "The values block of this unit does not declare \"tgas\", so try() always returns its fallback.\n\n" +
				"Did you mean \"tags\"?\n\n" + tryAdvice,
			line: 9,
		},
		{
			name:   "last argument of try",
			block:  tagsValuesBlock,
			inputs: "inputs = {\n  tags = try(values.tags, values.tgas)\n}\n",
			detail: "The values block of this unit does not declare \"tgas\".\n\n" +
				"Did you mean \"tags\"?\n\n" +
				"Fix the reference, or declare \"tgas\" in the values block.",
			line: 9,
		},
		{
			name:   "evaluated",
			block:  tagsValuesBlock,
			inputs: "inputs = {\n  name = values.typo\n}\n",
			detail: "The values block of this unit does not declare \"typo\".\n\n" +
				"Declared values: tags.\n\n" +
				"Fix the reference, or declare \"typo\" in the values block.",
			line: 9,
		},
		{
			name:   "evaluated string index",
			block:  tagsValuesBlock,
			inputs: "inputs = {\n  name = values[\"typo\"]\n}\n",
			detail: "The values block of this unit does not declare \"typo\".\n\n" +
				"Declared values: tags.\n\n" +
				"Fix the reference, or declare \"typo\" in the values block.",
			line: 9,
		},
		{
			name:   "string index",
			block:  tagsValuesBlock,
			inputs: "inputs = {\n  name = try(values[\"typo\"], \"\")\n}\n",
			detail: "The values block of this unit does not declare \"typo\", so try() always returns its fallback.\n\n" +
				"Declared values: tags.\n\n" + tryAdvice,
			line: 9,
		},
		{
			name:   "nested attribute",
			block:  tagsValuesBlock,
			inputs: "inputs = {\n  name = try(values.tgas.name, \"\")\n}\n",
			detail: "The values block of this unit does not declare \"tgas\", so try() always returns its fallback.\n\n" +
				"Did you mean \"tags\"?\n\n" + tryAdvice,
			line: 9,
		},
		{
			name:   "empty values block",
			block:  "values {}\n",
			inputs: "inputs = {\n  name = values.name\n}\n",
			detail: "The values block of this unit does not declare \"name\".\n\n" +
				"The values block declares no values.\n\n" +
				"Fix the reference, or declare \"name\" in the values block.",
			line: 4,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			result := runValidate(t, &validateRun{
				files:       map[string]string{"app/terragrunt.hcl": tc.block + "\n" + tc.inputs},
				experiments: []string{experiment.ValuesBlock},
			})

			diag := requireOnlyDiagnosticAt(t, result, "Undeclared value", "app/terragrunt.hcl", tc.line)
			assert.Equal(t, tc.detail, diag.Detail)
		})
	}
}

func TestRunValidateValuesReferenceHighlightsTheTraversal(t *testing.T) {
	t.Parallel()

	result := runValidate(t, &validateRun{
		files: map[string]string{
			"app/terragrunt.hcl": tagsValuesBlock + "\ninputs = {\n  tags = try(values.tgas, {})\n}\n",
		},
		experiments: []string{experiment.ValuesBlock},
	})

	diag := requireOnlyDiagnosticAt(t, result, "Undeclared value", "app/terragrunt.hcl", 9)
	assert.Equal(t, 14, diag.Range.Start.Column)
	assert.Equal(t, 25, diag.Range.End.Column)

	require.NotNil(t, diag.Snippet)
	assert.Equal(t, `  tags = try(values.tgas, {})`, diag.Snippet.Code)
	assert.Equal(t, 13, diag.Snippet.HighlightStartOffset)
	assert.Equal(t, 24, diag.Snippet.HighlightEndOffset)
}

func TestRunValidateReportsValueBlockOutsideValuesBlock(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		file string
		src  string
	}{
		{name: "unit", file: "app/terragrunt.hcl", src: "value \"name\" {\n  type = string\n}\n"},
		{name: "stack", file: "app/terragrunt.stack.hcl", src: "value \"env\" {\n  type = string\n}\n"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			result := runValidate(t, &validateRun{
				files:       map[string]string{tc.file: tc.src},
				experiments: []string{experiment.ValuesBlock},
			})

			requireOnlyDiagnosticAt(t, result, "Value block outside a values block", tc.file, 1)
		})
	}
}

func TestRunValidateValuesReferencesAcceptDeclaredValues(t *testing.T) {
	t.Parallel()

	result := runValidate(t, &validateRun{
		files: map[string]string{
			"app/terragrunt.hcl": tagsValuesBlock + `
locals {
  everything = values
  dynamic    = values[local.key]
  key        = "tags"
  scoped     = [for values in [{ typo = "a" }] : values.typo]
}

inputs = {
  tags = values.tags
  name = try(values.tags.name, "")
}
`,
		},
		experiments: []string{experiment.ValuesBlock},
	})

	require.NoError(t, result.err)
	assert.Empty(t, result.diags)
}

func TestRunValidateValuesReferencesInStack(t *testing.T) {
	t.Parallel()

	result := runValidate(t, &validateRun{
		files: map[string]string{
			"live/terragrunt.stack.hcl": `values {
  value "region" {
    default = "us-east-1"
  }
}

unit "app" {
  source = "../app"
  path   = "app"
  values = {
    region = values.regoin
  }
}
`,
			"app/terragrunt.hcl": "",
		},
		experiments: []string{experiment.ValuesBlock},
	})

	diag := requireOnlyDiagnosticAt(t, result, "Undeclared value", "live/terragrunt.stack.hcl", 11)
	assert.Equal(
		t,
		"The values block of this stack does not declare \"regoin\".\n\n"+
			"Did you mean \"region\"?\n\n"+
			"Fix the reference, or declare \"regoin\" in the values block.",
		diag.Detail,
	)
}

func TestRunValidateReportsInvalidValuesBlockOnce(t *testing.T) {
	t.Parallel()

	result := runValidate(t, &validateRun{
		files: map[string]string{
			"app/terragrunt.hcl": "values {\n  value \"tags\" {\n    type = text\n  }\n}\n",
		},
		experiments: []string{experiment.ValuesBlock},
	})

	requireOnlyDiagnosticAt(t, result, "Invalid type specification", "app/terragrunt.hcl", 3)
}

func TestRunValidateReportsDuplicateValuesBlockOnce(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		files map[string]string
		name  string
		file  string
	}{
		{
			name:  "unit",
			file:  "app/terragrunt.hcl",
			files: map[string]string{"app/terragrunt.hcl": "values {}\n\nvalues {}\n"},
		},
		{
			name: "stack",
			file: "live/terragrunt.stack.hcl",
			files: map[string]string{
				"live/terragrunt.stack.hcl": "values {}\n\nvalues {}\n",
				"app/terragrunt.hcl":        "",
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			result := runValidate(t, &validateRun{files: tc.files, experiments: []string{experiment.ValuesBlock}})

			requireOnlyDiagnosticAt(t, result, "Duplicate values block", tc.file, 3)
		})
	}
}

func TestRunValidateValuesReferencesOfUnitThatFailsToParse(t *testing.T) {
	t.Parallel()

	result := runValidate(t, &validateRun{
		files: map[string]string{
			"app/terragrunt.hcl": tagsValuesBlock + `
include "root" {
  path = "${get_terragrunt_dir()}/missing.hcl"
}

inputs = {
  tags = try(values.tgas, {})
}
`,
		},
		experiments: []string{experiment.ValuesBlock},
	})

	requireOnlyDiagnosticAt(t, result, "Undeclared value", "app/terragrunt.hcl", 13)
}

// TestRunValidateValuesReferencesOfStackThatFailsToParse pins that a stack whose parse fails still has its value reads
// checked.
func TestRunValidateValuesReferencesOfStackThatFailsToParse(t *testing.T) {
	t.Parallel()

	result := runValidate(t, &validateRun{
		files: map[string]string{
			"live/terragrunt.stack.hcl": `values {
  value "region" {
    type = string
  }
}

unit "app" {
  source = "../app"
  path   = "app"
  values = {
    region = values.regoin
  }
}
`,
			"app/terragrunt.hcl": "",
		},
		experiments: []string{experiment.ValuesBlock},
	})

	require.Error(t, result.err)

	summaries := make([]string, 0, len(result.diags))
	for _, diag := range result.diags {
		summaries = append(summaries, diag.Summary)
	}

	assert.ElementsMatch(t, []string{`Missing required value "region"`, "Undeclared value"}, summaries)
}

func TestRunValidateValuesReferencesUncheckedFiles(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		files map[string]string
		name  string
	}{
		{
			name: "no values block",
			files: map[string]string{
				"app/terragrunt.hcl": "inputs = {\n  name = try(values.typo, \"\")\n}\n",
			},
		},
		{
			name: "json",
			files: map[string]string{
				"app/terragrunt.hcl.json": `{
  "values": {
    "value": {
      "tags": {
        "type": "map(string)",
        "default": {}
      }
    }
  },
  "inputs": {
    "name": "${try(values.typo, \"\")}"
  }
}
`,
			},
		},
		{
			name: "included file outside the unit directory",
			files: map[string]string{
				"root.hcl": "inputs = {\n  name = try(values.typo, \"\")\n}\n",
				"app/terragrunt.hcl": tagsValuesBlock + `
include "root" {
  path = find_in_parent_folders("root.hcl")
}
`,
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			result := runValidate(t, &validateRun{files: tc.files, experiments: []string{experiment.ValuesBlock}})

			require.NoError(t, result.err)
			assert.Empty(t, result.diags)
		})
	}
}

func TestRunValidateValuesBlockRequiresExperiment(t *testing.T) {
	t.Parallel()

	result := runValidate(t, &validateRun{files: map[string]string{
		"app/terragrunt.hcl": tagsValuesBlock + "\ninputs = {\n  name = try(values.typo, \"\")\n}\n",
	}})

	var experimentErr config.ValuesBlockRequiresExperimentError

	require.ErrorAs(t, result.err, &experimentErr)
	assert.Equal(t, filepath.Join(result.root, "app", config.DefaultTerragruntConfigPath), experimentErr.ConfigPath)
	assert.Empty(t, result.diags)
}

// TestRunValidateValuesBlockRequiresValuesFile pins that a unit with a required value and no values file beside it
// fails validation, as a catalog unit does.
func TestRunValidateValuesBlockRequiresValuesFile(t *testing.T) {
	t.Parallel()

	result := runValidate(t, &validateRun{
		files: map[string]string{
			"catalog/units/app/terragrunt.hcl": "values {\n  value \"name\" {\n    type = string\n  }\n}\n",
		},
		experiments: []string{experiment.ValuesBlock},
	})

	requireOnlyDiagnosticAt(t, result, `Missing required value "name"`, "catalog/units/app/terragrunt.hcl", 1)
}

// TestRunValidateReportsEveryMissingRequiredValue pins that one diagnostic names every required value that nothing
// sets, with or without a values file.
func TestRunValidateReportsEveryMissingRequiredValue(t *testing.T) {
	t.Parallel()

	const unit = `values {
  value "dep" {}
  value "inc" {}
  value "other" {
    default = 1
  }
}
`

	testCases := []struct {
		files map[string]string
		name  string
		file  string
		line  int
	}{
		{
			name:  "no values file",
			files: map[string]string{"app/terragrunt.hcl": unit},
			file:  "app/terragrunt.hcl",
			line:  1,
		},
		{
			name: "values file that sets neither",
			files: map[string]string{
				"app/terragrunt.hcl":        unit,
				"app/terragrunt.values.hcl": "other = 2\n",
			},
			file: "app/terragrunt.values.hcl",
			line: 1,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			result := runValidate(t, &validateRun{files: tc.files, experiments: []string{experiment.ValuesBlock}})

			diag := requireOnlyDiagnosticAt(t, result, `Missing required values "dep", "inc"`, tc.file, tc.line)
			assert.Contains(t, diag.Detail, `"dep" (declared on line `)
			assert.Contains(t, diag.Detail, `"inc" (declared on line `)
		})
	}
}

// TestRunValidateValuesFileOfStack pins that a diagnostic in the values file of a stack is collected at that file.
func TestRunValidateValuesFileOfStack(t *testing.T) {
	t.Parallel()

	result := runValidate(t, &validateRun{
		files: map[string]string{
			"live/terragrunt.stack.hcl":  "values {\n  value \"region\" {\n    default = \"us-east-1\"\n  }\n}\n",
			"live/terragrunt.values.hcl": "region = \"eu-west-1\"\nregoin = \"eu-west-2\"\n",
		},
		experiments: []string{experiment.ValuesBlock},
	})

	requireOnlyDiagnosticAt(t, result, `Undeclared value "regoin"`, "live/terragrunt.values.hcl", 2)
}

// requireOnlyDiagnosticAt requires that result has one diagnostic, with summary, starting at line of file.
func requireOnlyDiagnosticAt(
	t *testing.T,
	result validateResult,
	summary, file string,
	line int,
) *diagnostic.Diagnostic {
	t.Helper()

	require.Error(t, result.err)
	require.Len(t, result.diags, 1)

	diag := result.diags[0]

	assert.Equal(t, summary, diag.Summary)
	require.NotNil(t, diag.Range)
	assert.Equal(t, filepath.Join(result.root, filepath.FromSlash(file)), diag.Range.Filename)
	assert.Equal(t, line, diag.Range.Start.Line)

	return diag
}
