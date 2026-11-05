package tips_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/tips"
	"github.com/gruntwork-io/terragrunt/internal/view"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"
	"github.com/hashicorp/hcl/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// inputsRead builds an InputsRead pointing at the first occurrence of expr in source, which is in file.
func inputsRead(
	t *testing.T,
	file, function string,
	place tips.InputsReadPlace,
	source, expr string,
) tips.InputsRead {
	t.Helper()

	require.Contains(t, source, expr)

	start := strings.Index(source, expr)
	end := start + len(expr)

	pos := func(offset int) hcl.Pos {
		before := source[:offset]
		line := strings.Count(before, "\n") + 1
		column := offset - strings.LastIndex(before, "\n")

		return hcl.Pos{Line: line, Column: column, Byte: offset}
	}

	return tips.InputsRead{
		File:     file,
		Function: function,
		Source:   []byte(source),
		Range:    hcl.Range{Filename: file, Start: pos(start), End: pos(end)},
		Place:    place,
	}
}

func TestInputsReadMessage(t *testing.T) {
	t.Parallel()

	const unitSource = `include "root" {
  path = find_in_parent_folders("root.hcl")
}

inputs = {
  message = yamldecode(file("../config.yml")).message
  policy  = templatefile("policy.json.tftpl", { role_arn = dependency.iam.outputs.role_arn })
}
`

	const stackSource = `unit "api" {
  autoinclude {
    inputs = {
      config = yamldecode(file(
        "config.yml",
      ))
    }
  }
}
`

	reads := []tips.InputsRead{
		inputsRead(t, "app/terragrunt.hcl", "file", tips.InputsReadInUnit, unitSource, `file("../config.yml")`),
		inputsRead(t, "app/terragrunt.hcl", "templatefile", tips.InputsReadInUnit, unitSource, `"policy.json.tftpl"`),
		inputsRead(
			t,
			"live/terragrunt.stack.hcl",
			"file",
			tips.InputsReadInAutoInclude,
			stackSource,
			"file(\n        \"config.yml\",\n      )",
		),
	}

	msg, err := tips.InputsReadMessage(view.NewHumanRender(venvtest.New(), true), reads)
	require.NoError(t, err)

	assert.Equal(
		t,
		"Terragrunt does not track files read in `inputs`, "+
			"so reading and Git filters miss the units and stacks that read them.\n"+
			"\n"+
			"Untracked file read in `inputs`\n"+
			" --> app/terragrunt.hcl:6:24\n"+
			"  |\n"+
			"6 |   message = yamldecode(file(\"../config.yml\")).message\n"+
			"  |                        ^^^^^^^^^^^^^^^^^^^^^\n"+
			"  |\n"+
			"  = Read the file in `locals` and reference the local here.\n"+
			"\n"+
			"Untracked file read in `inputs`\n"+
			" --> app/terragrunt.hcl:7:26\n"+
			"  |\n"+
			"7 |   policy  = templatefile(\"policy.json.tftpl\", { role_arn = dependency.iam.outputs.role_arn })\n"+
			"  |                          ^^^^^^^^^^^^^^^^^^^\n"+
			"  |\n"+
			"  = Mark the template as read with `mark_as_read` in `locals`, then pass the\n"+
			"    local as the path here.\n"+
			"\n"+
			"Untracked file read in an `autoinclude` block's `inputs`\n"+
			" --> live/terragrunt.stack.hcl:4:27\n"+
			"  |\n"+
			"4 |       config = yamldecode(file(\n"+
			"  |                           ^^^^^\n"+
			"5 |         \"config.yml\",\n"+
			"  |         ^^^^^^^^^^^^^\n"+
			"6 |       ))\n"+
			"  |       ^\n"+
			"  |\n"+
			"  = Read the file in the stack's `locals` and reference the local here.\n"+
			"\n"+
			"See https://docs.terragrunt.com/features/filter/attributes#reading-based-expressions",
		msg,
	)
}

func TestInputsReadMessageCapsListedReads(t *testing.T) {
	t.Parallel()

	const source = "inputs = {\n  message = file(\"config.yml\")\n}\n"

	reads := make([]tips.InputsRead, 0, 12)
	for i := range 12 {
		reads = append(reads, inputsRead(
			t,
			fmt.Sprintf("unit-%02d/terragrunt.hcl", i),
			"file",
			tips.InputsReadInUnit,
			source,
			`file("config.yml")`,
		))
	}

	msg, err := tips.InputsReadMessage(view.NewHumanRender(venvtest.New(), true), reads)
	require.NoError(t, err)

	assert.Contains(t, msg, " --> unit-09/terragrunt.hcl:2:13\n")
	assert.NotContains(t, msg, "unit-10/terragrunt.hcl")
	assert.Contains(t, msg, "\n\n... and 2 more\n\nSee https://")
}
