package scaffold_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/term"

	"github.com/gruntwork-io/terragrunt/internal/cli/commands/scaffold"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/internal/view/tui/form"
	"github.com/gruntwork-io/terragrunt/pkg/options"
	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
)

// fieldNames returns the names of fields in the order the form lists them.
func fieldNames(fields []form.Field) []string {
	names := make([]string, 0, len(fields))
	for i := range fields {
		names = append(names, fields[i].Name)
	}

	return names
}

// prepare runs Prepare against source and returns the plan, cleaned up when
// the test ends.
func prepare(t *testing.T, source string) *scaffold.Plan {
	t.Helper()

	outputDir := helpers.TmpDirWOSymlinks(t)

	opts, err := options.NewTerragruntOptionsForTest(filepath.Join(outputDir, "terragrunt.hcl"))
	require.NoError(t, err)

	opts.WorkingDir = outputDir
	opts.NonInteractive = true

	v := venv.OSVenv()

	plan, err := scaffold.Prepare(
		t.Context(), logger.CreateLogger(), v, opts, source, "",
	)
	require.NoError(t, err)

	t.Cleanup(func() { plan.Cleanup(v.FS) })

	return plan
}

// TestPlanFormFieldsForModule covers what the form asks for when scaffolding
// generates a configuration: the module's own variables.
func TestPlanFormFieldsForModule(t *testing.T) {
	t.Parallel()

	source := writeComponent(t, "modules/vpc", map[string]string{
		"main.tf": `variable "name" {
  type = string
}

variable "region" {
  type    = string
  default = "us-east-1"
}
`,
	})

	fields := prepare(t, source).FormFields()

	assert.Equal(t, []string{"name", "region"}, fieldNames(fields))
	assert.True(t, fields[0].Required, "a variable with no default must be filled in")
	assert.False(t, fields[1].Required)
}

// TestPlanFormFieldsForUnit covers what the form asks for when scaffolding
// copies: the `values.*` references the unit makes, which is what the copied
// configuration reads.
func TestPlanFormFieldsForUnit(t *testing.T) {
	t.Parallel()

	source := writeComponent(t, "units/app", map[string]string{
		"terragrunt.hcl": unitConfig,
	})

	fields := prepare(t, source).FormFields()

	assert.Equal(t, []string{"base_url", "name", "ref", "region"}, fieldNames(fields))
	assert.True(t, fields[0].Required)
	assert.False(t, fields[3].Required, "a reference behind try() carries its fallback")
}

func TestPlanFormFieldsEmptyForAComponentThatAsksForNothing(t *testing.T) {
	t.Parallel()

	source := writeComponent(t, "units/app", map[string]string{
		"terragrunt.hcl": "# nothing to fill in\n",
	})

	assert.Empty(t, prepare(t, source).FormFields())
}

// TestRunInteractiveNonInteractiveSkipsTheForm pins the --non-interactive
// contract: the same output the command produced before there was a form.
func TestRunInteractiveNonInteractiveSkipsTheForm(t *testing.T) {
	t.Parallel()

	source := writeComponent(t, "units/app", map[string]string{
		"terragrunt.hcl": unitConfig,
	})

	outputDir := helpers.TmpDirWOSymlinks(t)

	opts, err := options.NewTerragruntOptionsForTest(filepath.Join(outputDir, "terragrunt.hcl"))
	require.NoError(t, err)

	opts.WorkingDir = outputDir
	opts.NonInteractive = true

	require.NoError(t, scaffold.RunInteractive(
		t.Context(), logger.CreateLogger(), venv.OSVenv(), opts, source, "",
	))

	values := readFile(t, filepath.Join(outputDir, "terragrunt.values.hcl"))
	assert.Contains(t, values, `name     = "TODO"`, "every value is left for the user to fill in")
}

// TestRunInteractiveWithoutTerminalSkipsTheForm covers a run with no terminal
// to draw the form on, such as a CI job: scaffolding falls back to its
// placeholders rather than failing.
//
// It skips where stdin is a terminal, since a regression would open the form
// for real and block.
func TestRunInteractiveWithoutTerminalSkipsTheForm(t *testing.T) {
	t.Parallel()

	if term.IsTerminal(int(os.Stdin.Fd())) {
		t.Skip("stdin is a terminal; a regression would open the scaffold form for real")
	}

	source := writeComponent(t, "units/app", map[string]string{
		"terragrunt.hcl": unitConfig,
	})

	outputDir := helpers.TmpDirWOSymlinks(t)

	opts, err := options.NewTerragruntOptionsForTest(filepath.Join(outputDir, "terragrunt.hcl"))
	require.NoError(t, err)

	opts.WorkingDir = outputDir

	require.NoError(t, scaffold.RunInteractive(
		t.Context(), logger.CreateLogger(), venv.OSVenv(), opts, source, "",
	))

	assert.FileExists(t, filepath.Join(outputDir, "terragrunt.hcl"))
	assert.Contains(t, readFile(t, filepath.Join(outputDir, "terragrunt.values.hcl")), `"TODO"`)
}
