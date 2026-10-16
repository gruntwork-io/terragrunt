package test_test

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/tf"
	"github.com/gruntwork-io/terragrunt/internal/vexec"
	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testFixtureReportPath = "fixtures/report"
)

// TestTerragruntReportDisableSummary pins that --summary-disable drops the run summary, pass or fail.
func TestTerragruntReportDisableSummary(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name        string
		flag        string
		planExit    int
		wantSummary bool
	}{
		{name: "summary printed after a passing run", wantSummary: true},
		{name: "summary printed after a failing run", planExit: 1, wantSummary: true},
		{name: "summary disabled after a passing run", flag: " --summary-disable"},
		{name: "summary disabled after a failing run", flag: " --summary-disable", planExit: 1},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var planRuns atomic.Int32

			root := venvtest.Root("/repo")
			v := venvtest.New().
				WithFS(venvtest.NewFS(t, root, map[string]string{
					filepath.Join("unit", "terragrunt.hcl"): "",
					filepath.Join("unit", "main.tf"):        "",
				})).
				WithExec(vexec.NewMemExec(
					reportStubExec(root, tc.planExit, &planRuns),
					vexec.WithLookPath(reportStubLookPath),
				))

			stdout, _, err := helpers.RunTerragruntCommandWithOutputWithVenv(
				t,
				v,
				"terragrunt run --all plan --non-interactive --working-dir "+root+tc.flag,
			)

			switch tc.planExit {
			case 0:
				require.NoError(t, err)
			default:
				require.Error(t, err, "a failing plan must fail the run")
			}

			assert.Equal(t, int32(1), planRuns.Load(), "the unit's plan must have run for the summary to describe it")

			if tc.wantSummary {
				assert.Contains(t, stdout, "Run Summary", "a run --all prints the summary unless told not to")

				return
			}

			assert.NotContains(t, stdout, "Run Summary", "--summary-disable must drop the summary")
		})
	}
}

// lineType represents the type of line we're processing
type lineType int

const (
	categoryHeaderLine lineType = iota
	unitLine
	otherLine
)

// getLineType determines what type of line we're dealing with
func getLineType(line string, inCategory bool) lineType {
	trimmed := strings.TrimSpace(line)

	// Check if this is a category header line (ends with a count in parentheses)
	if strings.Contains(line, "(") && strings.Contains(line, ")") &&
		(strings.Contains(line, "Succeeded") || strings.Contains(line, "Failed") ||
			strings.Contains(line, "Early Exits") || strings.Contains(line, "Excluded")) {
		return categoryHeaderLine
	}

	// Check if this is a unit line within a category
	if inCategory && strings.HasPrefix(line, "      ") && trimmed != "" {
		return unitLine
	}

	return otherLine
}

// sortLinesWithinCategories sorts the unit lines within each category alphabetically
// to make the test deterministic regardless of actual execution timing
func sortLinesWithinCategories(input string) string {
	lines := strings.Split(input, "\n")

	var (
		result               []string
		currentCategoryLines []string
	)

	inCategory := false

	for _, line := range lines {
		switch getLineType(line, inCategory) {
		case categoryHeaderLine:
			// If we were in a category, sort and add those lines first
			if inCategory && len(currentCategoryLines) > 0 {
				sort.Strings(currentCategoryLines)
				result = append(result, currentCategoryLines...)
				currentCategoryLines = nil
			}
			// Add the category header
			result = append(result, line)
			inCategory = true
		case unitLine:
			// This is a unit line within a category
			currentCategoryLines = append(currentCategoryLines, line)
		case otherLine:
			// If we were in a category, sort and add those lines first
			if inCategory && len(currentCategoryLines) > 0 {
				sort.Strings(currentCategoryLines)
				result = append(result, currentCategoryLines...)
				currentCategoryLines = nil
				inCategory = false
			}
			// Add non-category lines as-is
			result = append(result, line)
		}
	}

	// Handle any remaining category lines
	if inCategory && len(currentCategoryLines) > 0 {
		sort.Strings(currentCategoryLines)
		result = append(result, currentCategoryLines...)
	}

	return strings.Join(result, "\n")
}

// createReportTestUnit creates a unit directory with terragrunt.hcl and main.tf files.
func createReportTestUnit(t *testing.T, dir, comment string) {
	t.Helper()

	err := os.MkdirAll(dir, 0755)
	require.NoError(t, err)

	err = os.WriteFile(filepath.Join(dir, "terragrunt.hcl"), []byte(comment), 0644)
	require.NoError(t, err)

	err = os.WriteFile(filepath.Join(dir, "main.tf"), []byte(`# Minimal terraform config`), 0644)
	require.NoError(t, err)
}

// reportStubExec stands in for OpenTofu and git; plan exits with planExit and is counted in planRuns.
func reportStubExec(root string, planExit int, planRuns *atomic.Int32) vexec.Handler {
	return func(_ context.Context, inv vexec.Invocation) vexec.Result {
		name := filepath.Base(inv.Name)
		isTofu := name == "tofu" || name == "terraform"

		switch {
		case isTofu && slices.ContainsFunc(inv.Args, isReportVersionArg):
			return vexec.Result{Stdout: []byte("OpenTofu v1.10.0\non linux_amd64\n")}
		case isTofu && slices.Contains(inv.Args, tf.CommandNamePlan):
			planRuns.Add(1)

			return vexec.Result{ExitCode: planExit}
		case name == "git" && slices.Contains(inv.Args, "--show-toplevel"):
			return vexec.Result{Stdout: []byte(root + "\n")}
		}

		return vexec.Result{}
	}
}

func reportStubLookPath(file string) (string, error) {
	return "/usr/bin/" + file, nil
}

func isReportVersionArg(arg string) bool {
	return arg == tf.FlagNameVersion || arg == tf.CommandNameVersion
}
