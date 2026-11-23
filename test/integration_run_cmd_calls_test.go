package test_test

import (
	"context"
	"maps"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/tf"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/internal/vexec"
	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// runCmdCallsFixture calls run_cmd from each place a config can: the locals, inputs, and generate blocks of an included
// root, the locals and feature flags of units, a file read with read_terragrunt_config, a dependency block, and a
// dependency the parse reads the config of. Each call runs `mark <place>`. The calls whose place ends in no-cache pass
// --terragrunt-no-cache, so they run each time Terragrunt evaluates them, and the call whose place ends in global
// passes --terragrunt-global-cache.
var runCmdCallsFixture = map[string]string{
	"root.hcl": `
locals {
  root          = run_cmd("--terragrunt-quiet", "mark", "root-local")
  root_no_cache = run_cmd("--terragrunt-quiet", "--terragrunt-no-cache", "mark", "root-local-no-cache")
  root_global   = run_cmd("--terragrunt-quiet", "--terragrunt-global-cache", "mark", "root-local-global")
}

inputs = {
  root = run_cmd("--terragrunt-quiet", "mark", "root-input")
}

generate "marker" {
  path      = "marker.tf"
  if_exists = "overwrite"
  contents  = "# ${run_cmd("--terragrunt-quiet", "mark", "root-generate")}"
}
`,
	"common.hcl": `
locals {
  common          = run_cmd("--terragrunt-quiet", "mark", "common-local")
  common_no_cache = run_cmd("--terragrunt-quiet", "--terragrunt-no-cache", "mark", "common-local-no-cache")
}
`,
	filepath.Join("vpc", "terragrunt.hcl"): `
include "root" {
  path = find_in_parent_folders("root.hcl")
}

locals {
  vpc          = run_cmd("--terragrunt-quiet", "mark", "vpc-local")
  vpc_no_cache = run_cmd("--terragrunt-quiet", "--terragrunt-no-cache", "mark", "vpc-local-no-cache")
}
`,
	filepath.Join("vpc", "main.tf"): "",
	filepath.Join("app", "terragrunt.hcl"): `
include "root" {
  path   = find_in_parent_folders("root.hcl")
  expose = true
}

feature "flag" {
  default = run_cmd("--terragrunt-quiet", "mark", "app-feature")
}

locals {
  app          = run_cmd("--terragrunt-quiet", "mark", "app-local")
  app_no_cache = run_cmd("--terragrunt-quiet", "--terragrunt-no-cache", "mark", "app-local-no-cache")
  common       = read_terragrunt_config(find_in_parent_folders("common.hcl"))
}

dependency "vpc" {
  config_path = "../vpc"

  mock_outputs = {
    id = run_cmd("--terragrunt-quiet", "mark", "app-dependency-mock")
  }
}

inputs = {
  app    = run_cmd("--terragrunt-quiet", "mark", "app-input")
  vpc_id = dependency.vpc.outputs.id
}
`,
	filepath.Join("app", "main.tf"): "",
}

// pins how many times a command runs each run_cmd call of [runCmdCallsFixture]: a cached call once for each
// directory that evaluates it, and an uncached call at most once for each time Terragrunt evaluates it
func TestRunCmdCallsWithRacing(t *testing.T) {
	t.Parallel()

	runCached := map[string]int{
		"app-dependency-mock": 1,
		"app-feature":         1,
		"app-input":           1,
		"app-local":           1,
		"common-local":        1,
		"root-generate":       2,
		"root-input":          2,
		"root-local":          2,
		"root-local-global":   1,
		"vpc-local":           1,
	}

	testCases := []struct {
		cached      map[string]int
		maxUncached map[string]int
		name        string
		command     string
		dir         string
	}{
		{
			name:    "run all",
			command: "run --all plan --non-interactive",
			cached:  runCached,
			maxUncached: map[string]int{
				"app-local-no-cache":    2,
				"common-local-no-cache": 2,
				"root-local-no-cache":   10,
				"vpc-local-no-cache":    6,
			},
		},
		{
			name:    "run",
			command: "run plan --non-interactive",
			dir:     "app",
			cached:  runCached,
			maxUncached: map[string]int{
				"app-local-no-cache":    2,
				"common-local-no-cache": 2,
				"root-local-no-cache":   9,
				"vpc-local-no-cache":    4,
			},
		},
		{
			name:    "find dependencies",
			command: "find --json --dependencies",
			cached: map[string]int{
				"app-dependency-mock": 1,
				"app-feature":         1,
				"app-local":           1,
				"common-local":        1,
				"root-generate":       1,
				"root-input":          1,
				"root-local":          2,
				"root-local-global":   1,
				"vpc-local":           1,
			},
			maxUncached: map[string]int{
				"app-local-no-cache":    1,
				"common-local-no-cache": 1,
				"root-local-no-cache":   2,
				"vpc-local-no-cache":    1,
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			root := venvtest.Root("/repo")
			stub := &runCmdStub{root: root, marks: map[string]int{}}

			v := venv.MemVenv().
				WithFS(venvtest.NewFS(t, root, runCmdCallsFixture)).
				WithExec(vexec.NewMemExec(stub.handler(), vexec.WithLookPath(reportStubLookPath)))

			_, _, err := helpers.RunTerragruntCommandWithOutputWithVenv(
				t,
				v,
				"terragrunt "+tc.command+" --working-dir "+filepath.Join(root, tc.dir),
			)
			require.NoError(t, err)

			assert.Empty(t, stub.unexpectedCommands())

			cached := stub.markCounts()
			uncached := map[string]int{}

			maps.DeleteFunc(cached, func(place string, count int) bool {
				if strings.HasSuffix(place, "-no-cache") {
					uncached[place] = count

					return true
				}

				return false
			})

			assert.Equal(t, tc.cached, cached)

			// How often concurrent parses share an evaluation depends on timing, so an uncached call gets a ceiling.
			assert.ElementsMatch(t, slices.Collect(maps.Keys(tc.maxUncached)), slices.Collect(maps.Keys(uncached)))

			for place, most := range tc.maxUncached {
				assert.LessOrEqual(t, uncached[place], most, place)
			}
		})
	}
}

// runCmdStub answers the commands a run of [runCmdCallsFixture] spawns and counts the `mark` commands by place.
type runCmdStub struct {
	marks      map[string]int
	root       string
	unexpected []string
	mu         sync.Mutex
}

func (s *runCmdStub) handler() vexec.Handler {
	return func(_ context.Context, inv vexec.Invocation) vexec.Result {
		name := filepath.Base(inv.Name)
		isTofu := name == "tofu" || name == "terraform"

		switch {
		case name == "mark" && len(inv.Args) == 1:
			s.mu.Lock()
			s.marks[inv.Args[0]]++
			s.mu.Unlock()

			return vexec.Result{Stdout: []byte(inv.Args[0])}
		case isTofu && slices.ContainsFunc(inv.Args, isReportVersionArg):
			return vexec.Result{Stdout: []byte("OpenTofu v1.10.0\non linux_amd64\n")}
		case isTofu && slices.Contains(inv.Args, tf.CommandNameOutput):
			return vexec.Result{Stdout: []byte("{}")}
		case isTofu && (slices.Contains(inv.Args, tf.CommandNameInit) || slices.Contains(inv.Args, tf.CommandNamePlan)):
			return vexec.Result{}
		case name == "git" && slices.Contains(inv.Args, "--show-toplevel"):
			return vexec.Result{Stdout: []byte(s.root + "\n")}
		}

		command := name + " " + strings.Join(inv.Args, " ")

		s.mu.Lock()
		s.unexpected = append(s.unexpected, command)
		s.mu.Unlock()

		return vexec.Result{ExitCode: 1, Stderr: []byte("unexpected subprocess: " + command + "\n")}
	}
}

func (s *runCmdStub) markCounts() map[string]int {
	s.mu.Lock()
	defer s.mu.Unlock()

	return maps.Clone(s.marks)
}

func (s *runCmdStub) unexpectedCommands() []string {
	s.mu.Lock()
	defer s.mu.Unlock()

	return slices.Clone(s.unexpected)
}
