package mcp_test

import (
	"context"
	"fmt"
	"io/fs"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"

	tgmcp "github.com/gruntwork-io/terragrunt/internal/cli/commands/mcp"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/internal/vexec"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type planChanges struct {
	Add     int `json:"add"`
	Change  int `json:"change"`
	Destroy int `json:"destroy"`
	Import  int `json:"import"`
	Forget  int `json:"forget"`
	Outputs int `json:"outputs"`
}

type planUnit struct {
	Changes         *planChanges `json:"changes"`
	Path            string       `json:"path"`
	Result          string       `json:"result"`
	Stdout          string       `json:"stdout"`
	PlanJSON        string       `json:"plan_json"`
	StdoutTruncated bool         `json:"stdout_truncated"`
}

type planOutput struct {
	Units    []planUnit `json:"units"`
	Degraded []string   `json:"degraded"`
}

// planWithChanges is the plan a unit with changes prints to a terminal.
const planWithChanges = "OpenTofu will perform the following actions:\n" +
	"  # terraform_data.this will be created\n" +
	"\x1b[1mPlan:\x1b[0m 1 to import, 2 to add, 0 to change, 1 to destroy.\n"

const planWithoutChanges = "No changes. Your infrastructure matches the configuration.\n"

// planJSONWithChanges is the plan JSON of a unit that creates one resource,
// imports and updates another, replaces a third, and changes an output.
const planJSONWithChanges = `{"resource_changes":[` +
	`{"address":"a.one","change":{"actions":["create"]}},` +
	`{"address":"a.two","change":{"actions":["update"],"importing":{"id":"two"}}},` +
	`{"address":"a.three","change":{"actions":["delete","create"]}}],` +
	`"output_changes":{"id":{"actions":["update"]}}}` + "\n"

const planJSONWithoutChanges = `{"resource_changes":[{"address":"b.one","change":{"actions":["no-op"]}}],` +
	`"output_changes":{"id":{"actions":["no-op"]}}}` + "\n"

// planChangesInJSON is what planJSONWithChanges counts to.
var planChangesInJSON = &planChanges{Add: 2, Change: 1, Destroy: 1, Import: 1, Outputs: 1}

// fakePlanVenv returns a venv over the real filesystem whose tofu prints
// planFor(dir) for a plan run in dir, jsonFor(dir) for a show of its plan
// file, and nothing for init.
func fakePlanVenv(planFor, jsonFor func(dir string) string) *venv.Venv {
	return venvtest.NewWithOSFS().
		WithHandler(func(_ context.Context, inv vexec.Invocation) vexec.Result {
			switch inv.Args[0] {
			case "plan":
				return vexec.Result{Stdout: []byte(planFor(inv.Dir))}
			case "show":
				return vexec.Result{Stdout: []byte(jsonFor(inv.Dir))}
			case "init":
				return vexec.Result{}
			}

			return vexec.Result{Stdout: []byte("OpenTofu v1.9.0\n")}
		})
}

// newPlanTree writes units with nothing but a module file.
func newPlanTree(t *testing.T, units ...string) string {
	t.Helper()

	files := map[string]string{}
	for _, unit := range units {
		files[unit+"/terragrunt.hcl"] = ""
		files[unit+"/main.tf"] = ""
	}

	return writeTree(t, files)
}

// unitOf names the unit a plan runs for from the directory it runs in.
func unitOf(dir, root string) string {
	rel, _, _ := strings.Cut(
		strings.TrimPrefix(dir, root+string(filepath.Separator)),
		string(filepath.Separator),
	)

	return rel
}

func planUnitByPath(t *testing.T, out planOutput, path string) planUnit {
	t.Helper()

	for _, unit := range out.Units {
		if unit.Path == path {
			return unit
		}
	}

	require.FailNowf(t, "unit missing", "no %s in %+v", path, out.Units)

	return planUnit{}
}

// TestPlanWritesEachUnitsPlanToAFileOfItsOwn pins that every unit's plan text
// goes to a file of its own, readable by its owner alone and free of color
// codes, and that its plan JSON and the counts in it come back with the
// result.
func TestPlanWritesEachUnitsPlanToAFileOfItsOwn(t *testing.T) {
	t.Parallel()

	dir := newPlanTree(t, "a", "b")

	v := fakePlanVenv(func(runDir string) string {
		if unitOf(runDir, dir) == "a" {
			return "plan of a\n" + planWithChanges
		}

		return "plan of b\n" + planWithoutChanges
	}, func(runDir string) string {
		if unitOf(runDir, dir) == "a" {
			return planJSONWithChanges
		}

		return planJSONWithoutChanges
	})

	session := newTestSessionWith(
		t,
		dir,
		v,
		func(o *tgmcp.Options) { o.Allow = []string{"exec"} },
		nil,
	)

	var out planOutput

	callTool(t, session, "plan", map[string]any{}, &out)

	a := planUnitByPath(t, out, "a")
	b := planUnitByPath(t, out, "b")

	assert.Equal(t, planChangesInJSON, a.Changes)
	assert.Equal(t, &planChanges{}, b.Changes, "a plan with no changes counts none")

	fsys := vfs.NewOSFS()

	aText, err := vfs.ReadFile(fsys, a.Stdout)
	require.NoError(t, err)
	assert.Equal(
		t,
		"plan of a\n"+strings.ReplaceAll(
			strings.ReplaceAll(planWithChanges, "\x1b[1m", ""),
			"\x1b[0m",
			"",
		),
		string(aText),
		"the file must contain a's plan alone, without color codes",
	)

	bText, err := vfs.ReadFile(fsys, b.Stdout)
	require.NoError(t, err)
	assert.Equal(t, "plan of b\n"+planWithoutChanges, string(bText))

	aJSON, err := vfs.ReadFile(fsys, a.PlanJSON)
	require.NoError(t, err)
	assert.JSONEq(t, planJSONWithChanges, string(aJSON), "the plan JSON is what show -json printed")

	if runtime.GOOS == "windows" {
		return
	}

	info, err := fsys.Stat(a.Stdout)
	require.NoError(t, err)
	assert.Equal(
		t,
		fs.FileMode(0o600),
		info.Mode().Perm(),
		"plan text is for the server's user alone",
	)

	info, err = fsys.Stat(filepath.Dir(a.Stdout))
	require.NoError(t, err)
	assert.Equal(t, fs.FileMode(0o700), info.Mode().Perm())
}

// TestPlanKeepsItsFilesUntilTheServerStops pins that a plan leaves the files
// of the plans before it alone, and that the server removes them all when it
// stops.
func TestPlanKeepsItsFilesUntilTheServerStops(t *testing.T) {
	t.Parallel()

	dir := newPlanTree(t, "a")
	fsys := vfs.NewOSFS()

	var latest string

	// Registered before the session, so it runs after the session closes.
	t.Cleanup(func() {
		assert.False(t, vfs.Exists(fsys, filepath.Dir(filepath.Dir(latest))),
			"the server must remove its stdout files when it stops")
	})

	v := fakePlanVenv(
		func(string) string { return planWithChanges },
		func(string) string { return planJSONWithChanges },
	)
	session := newTestSessionWith(
		t,
		dir,
		v,
		func(o *tgmcp.Options) { o.Allow = []string{"exec"} },
		nil,
	)

	var first, second planOutput

	callTool(t, session, "plan", map[string]any{}, &first)
	require.FileExists(t, planUnitByPath(t, first, "a").Stdout)

	callTool(t, session, "plan", map[string]any{}, &second)

	latest = planUnitByPath(t, second, "a").Stdout

	assert.FileExists(
		t,
		planUnitByPath(t, first, "a").Stdout,
		"a new plan leaves the last plan's files alone",
	)
	assert.FileExists(t, latest)
}

// TestPlanCutsAPlanThatOutgrowsItsFile pins that a unit's plan text keeps its
// end and loses its start once it passes the per-unit cap, and says so. The
// cut can fall in the text that reached the raw file or in the text that only
// the ring buffer saw.
func TestPlanCutsAPlanThatOutgrowsItsFile(t *testing.T) {
	t.Parallel()

	const (
		capBytes  = 8 << 20
		lineBytes = 1 << 10
		note      = "[stdout truncated: Terragrunt keeps the last 8388608 bytes per unit]\n\n"
	)

	tests := []struct {
		name  string
		lines int
	}{
		{name: "cut in the raw file", lines: capBytes/lineBytes + 1},
		{name: "cut in the ring buffer", lines: 5 * capBytes / lineBytes / 2},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			lines := make([]string, tc.lines)
			for i := range lines {
				lines[i] = fmt.Sprintf("%07d %s\n", i, strings.Repeat("x", lineBytes-9))
			}

			huge := strings.Join(lines, "") + planWithChanges

			dir := newPlanTree(t, "a")
			v := fakePlanVenv(
				func(string) string { return huge },
				func(string) string { return planJSONWithChanges },
			)
			session := newTestSessionWith(
				t,
				dir,
				v,
				func(o *tgmcp.Options) { o.Allow = []string{"exec"} },
				nil,
			)

			var out planOutput

			callTool(t, session, "plan", map[string]any{}, &out)

			a := planUnitByPath(t, out, "a")
			assert.True(t, a.StdoutTruncated)
			assert.Equal(t, planChangesInJSON, a.Changes)
			assert.NoFileExists(t, a.Stdout+".raw", "the raw file is removed once the plan ends")

			text, err := vfs.ReadFile(vfs.NewOSFS(), a.Stdout)
			require.NoError(t, err)
			require.True(t, strings.HasPrefix(string(text), note), "the file opens with the note")

			body := string(text[len(note):])
			require.LessOrEqual(t, len(body), capBytes)
			require.Greater(
				t,
				len(body),
				capBytes-2*lineBytes,
				"the window is at most one line short of the cap",
			)

			first, err := strconv.Atoi(body[:7])
			require.NoError(t, err, "the body starts on a whole line")

			want := strings.Join(
				lines[first:],
				"",
			) + strings.NewReplacer("\x1b[1m", "", "\x1b[0m", "").
				Replace(planWithChanges)
			require.Len(
				t,
				body,
				len(want),
				"the body is the last lines of the plan from line %d",
				first,
			)
			assert.Equal(
				t,
				want,
				body,
				"the body is the last lines of the plan in order, from line %d",
				first,
			)
		})
	}
}

// TestApplyAndDestroyWriteEachUnitsStdoutToAFileOfItsOwn pins that an apply
// and a destroy write each unit's stdout to a file under a directory named
// for the command, as a plan does, and that neither counts changes.
func TestApplyAndDestroyWriteEachUnitsStdoutToAFileOfItsOwn(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		stdout string
	}{
		{
			name:   "apply",
			stdout: "\x1b[1mApply complete!\x1b[0m Resources: 1 added, 0 changed, 0 destroyed.\n",
		},
		{name: "destroy", stdout: "Destroy complete! Resources: 1 destroyed.\n"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			dir := newPlanTree(t, "a", "b")

			v := venvtest.NewWithOSFS().
				WithHandler(func(_ context.Context, inv vexec.Invocation) vexec.Result {
					switch inv.Args[0] {
					case tc.name:
						return vexec.Result{Stdout: []byte(unitOf(inv.Dir, dir) + ": " + tc.stdout)}
					case "init":
						return vexec.Result{}
					}

					return vexec.Result{Stdout: []byte("OpenTofu v1.9.0\n")}
				})

			var prompts []string

			session := newAnsweringSession(t, dir, v, "accept", &prompts, func(o *tgmcp.Options) {
				o.Allow = []string{"exec"}
				o.AllowApply = true
			})

			var out planOutput

			callTool(t, session, tc.name, map[string]any{}, &out)
			require.Len(t, prompts, 1, "the run must ask once")

			for _, unit := range []string{"a", "b"} {
				res := planUnitByPath(t, out, unit)
				assert.Nil(t, res.Changes, "only a plan counts changes")
				assert.Empty(t, res.PlanJSON, "only a plan has a plan file")
				assert.True(
					t,
					strings.HasPrefix(filepath.Base(filepath.Dir(res.Stdout)), tc.name+"-"),
					"%s is under a directory named for the command",
					res.Stdout,
				)

				text, err := vfs.ReadFile(vfs.NewOSFS(), res.Stdout)
				require.NoError(t, err)
				assert.Equal(
					t,
					unit+": "+strings.NewReplacer("\x1b[1m", "", "\x1b[0m", "").Replace(tc.stdout),
					string(text),
					"the file must contain %s's stdout alone, without color codes",
					unit,
				)
			}
		})
	}
}
