//go:build tf

package test_test

import (
	"bytes"
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gruntwork-io/terragrunt/internal/vexec"
	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTFDependencyStackGenerateRecursesWithoutState(t *testing.T) {
	t.Parallel()

	for _, shape := range []string{"transitive", "diamond", "nested", "initial traversal"} {
		t.Run(shape, func(t *testing.T) {
			t.Parallel()
			f := newDependencyStackFixture(t, true)
			// Generation must discover dependencies without evaluating their outputs.
			f.write(".sources/db/terragrunt.hcl", "inputs = { marker = \"new\" }")
			f.write(
				"middle/terragrunt.stack.hcl",
				fmt.Sprintf("unit \"middle\" {\n source = %q\n path = \"middle\"\n}\n", filepath.Join(f.root, ".sources/middle")),
			)
			f.write(
				".sources/middle/terragrunt.hcl",
				fmt.Sprintf(
					"dependency \"producer\" {\n config_path = %q\n}\ninputs = { marker = dependency.producer.outputs.db.marker }\n",
					filepath.Join(f.root, "producer"),
				),
			)
			consumer := fmt.Sprintf(
				"unit \"app\" {\n source = %q\n path = \"app\"\n values = { producer = %q }\n}\n",
				filepath.Join(f.root, ".sources/app"),
				filepath.Join(f.root, "middle"),
			)
			dir := "consumer"
			expected := []string{
				"middle/.terragrunt-stack/middle/terragrunt.hcl",
				"producer/.terragrunt-stack/db/terragrunt.hcl",
			}

			switch shape {
			case "diamond":
				consumer += fmt.Sprintf(
					"unit \"other\" {\n source = %q\n path = \"other\"\n values = { producer = %q }\n}\n",
					filepath.Join(f.root, ".sources/app"),
					filepath.Join(f.root, "producer"),
				)
			case "nested":
				f.write(
					"middle/terragrunt.stack.hcl",
					fmt.Sprintf("stack \"nested\" {\n source = %q\n path = \"nested\"\n}\n", filepath.Join(f.root, "nested-source")),
				)
				f.write(
					"nested-source/terragrunt.stack.hcl",
					fmt.Sprintf("unit \"middle\" {\n source = %q\n path = \"middle\"\n}\n", filepath.Join(f.root, ".sources/middle")),
				)

				expected[0] = "middle/.terragrunt-stack/nested/.terragrunt-stack/middle/terragrunt.hcl"
			case "initial traversal":
				dir = "."
			}

			f.write("consumer/terragrunt.stack.hcl", consumer)
			f.v.Exec = vexec.NewMemExec(func(_ context.Context, inv vexec.Invocation) vexec.Result {
				if len(inv.Args) == 1 && inv.Args[0] == "-version" {
					return vexec.Result{Stdout: []byte("OpenTofu v1.13.2\n")}
				}

				assert.Fail(t, "stack generate must not execute state or plan commands", "%s %v", inv.Name, inv.Args)

				return vexec.Result{Err: fmt.Errorf("unexpected subprocess %s", inv.Name)}
			})
			_, stderr, err := f.run(dir, "stack generate")
			require.NoError(t, err, "%s", stderr)

			for _, path := range expected {
				assert.FileExists(t, filepath.Join(f.root, path))
			}

			assert.Equal(
				t,
				1,
				strings.Count(stderr, "Generating unit db from "),
				"shared producer must be generated once per command",
			)

			if dir != "." {
				assert.NoDirExists(t, filepath.Join(f.root, "unrelated/.terragrunt-stack"))
			}
		})
	}
}

func TestTFDependencyStackGenerateReportsCycle(t *testing.T) {
	t.Parallel()
	f := newDependencyStackFixture(t, true)
	f.write(
		".sources/db/terragrunt.hcl",
		fmt.Sprintf("dependency \"consumer\" {\n config_path = %q\n}\n", filepath.Join(f.root, "consumer")),
	)

	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()

	var stdout, stderr bytes.Buffer

	err := helpers.RunTerragruntCommandWithVenv(t, ctx, f.v.WithWriter(&stdout).WithErrWriter(&stderr),
		"terragrunt --non-interactive --working-dir "+filepath.Join(f.root, "consumer")+" stack generate")
	require.Error(t, err)
	require.NoError(t, ctx.Err(), "cycle detection must terminate before the deadline")

	if err != nil {
		assert.Contains(t, strings.ToLower(err.Error()), "cycle")
	}
}

func TestTFRunAllDiscoversStackDependencyFromStandaloneUnit(t *testing.T) {
	t.Parallel()

	for _, stale := range []bool{false, true} {
		t.Run(fmt.Sprintf("stale=%t", stale), func(t *testing.T) {
			t.Parallel()

			f := newDependencyStackFixture(t, true)
			if stale {
				f.generate("producer")
			}

			f.producerSource("new")
			f.write("standalone/terragrunt.hcl", fmt.Sprintf(`dependency "producer" {
 config_path = %q
}
inputs = { marker = dependency.producer.outputs.db.marker }
`, filepath.Join(f.root, "producer")))
			f.write("standalone/main.tf", `variable "marker" { type = string }
output "consumed" { value = var.marker }
`)
			stdout, stderr, err := f.run("standalone", "run --all --queue-include-external -- plan")
			require.NoError(t, err, "%s", stderr)
			assert.Contains(t, stdout, `consumed = "new"`)
			assert.Equal(t, 1, strings.Count(stderr, "Generating unit db from "))
			assert.NoDirExists(t, filepath.Join(f.root, "consumer/.terragrunt-stack"))
			assert.NoDirExists(t, filepath.Join(f.root, "unrelated/.terragrunt-stack"))
		})
	}
}

func TestTFDependencyStacksGenerateBeforeSelectingUnits(t *testing.T) {
	t.Parallel()

	for _, state := range []string{"missing", "missing inside generated tree", "stale", "changed reference", "missing generated value", "obsolete unit parse error"} {
		for _, includeExternal := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/include_external=%t", state, includeExternal), func(t *testing.T) {
				t.Parallel()

				changedReference := state == "changed reference" || state == "obsolete unit parse error"
				insideGeneratedTree := changedReference || state == "missing inside generated tree" ||
					state == "missing generated value"
				f := newDependencyStackFixture(t, true)
				f.v.Env["TG_USE_PARTIAL_PARSE_CONFIG_CACHE"] = "true"
				f.write(".sources/relay/terragrunt.hcl", `terraform { source = "." }
dependencies { paths = [values.next] }
inputs = { marker = values.marker }
`)
				f.write(".sources/relay/main.tf", `variable "marker" { type = string }
output "relay_marker" { value = var.marker }
`)

				writeStack := func(name, next, marker string) {
					f.write(name+"/terragrunt.stack.hcl", fmt.Sprintf(`unit "relay" {
 source = %q
 path = "relay"
 values = { next = %q, marker = %q }
}
`, filepath.ToSlash(filepath.Join(f.root, ".sources/relay")), filepath.ToSlash(filepath.Join(f.root, next)), marker))
				}

				f.write("terminal/terragrunt.hcl", "")
				f.write("terminal/main.tf", `output "terminal" { value = true }`)
				writeStack("first", "middle", "old")
				writeStack("middle", "last", "old")
				writeStack("last", "terminal", "old")
				writeStack("obsolete", "terminal", "obsolete")

				if changedReference {
					writeStack("first", "obsolete", "old")
				}

				if state != "missing" && state != "missing inside generated tree" {
					for _, stack := range []string{"first", "middle", "last", "obsolete"} {
						f.generate(stack)
					}
				}

				if state == "changed reference" {
					f.write("obsolete/terragrunt.stack.hcl", `unit "relay" {
 source = "./missing-source"
 path = "relay"
}`)
				}

				if state == "obsolete unit parse error" {
					require.NoError(t, f.v.FS.Remove(filepath.Join(f.root, "obsolete/terragrunt.stack.hcl")))
					f.write("obsolete/terragrunt.hcl", `locals { marker = missing.value }`)
				}

				writeStack("first", "middle", "new")
				writeStack("middle", "last", "new")
				writeStack("last", "terminal", "new")

				entryDir, runDir := "entry", "entry"
				command := fmt.Sprintf("run --all --queue-include-external=%t -- plan", includeExternal)

				if insideGeneratedTree {
					entryDir, runDir = "first/.terragrunt-stack/consumer", "first/.terragrunt-stack"
					command = fmt.Sprintf("run --all --filter 'source=*' --queue-include-external=%t -- plan", includeExternal)
				}

				f.write(entryDir+"/terragrunt.hcl", fmt.Sprintf(`terraform { source = "." }
dependencies { paths = [%q] }
`, filepath.ToSlash(filepath.Join(f.root, "first"))))
				f.write(entryDir+"/main.tf", `output "consumer" { value = true }`)

				if state == "missing generated value" {
					f.write("first/.terragrunt-stack/relay/terragrunt.values.hcl", `marker = "old"`)
				}

				stdout, stderr, err := f.run(runDir, command)

				require.NoError(t, err, "%s", stderr)
				assert.NotContains(t, stderr, "Error:")

				for _, stack := range []string{"first", "middle", "last"} {
					assert.Contains(t, f.read(stack+"/.terragrunt-stack/relay/terragrunt.values.hcl"), `"new"`)
				}

				assert.Contains(t, f.read("first/.terragrunt-stack/relay/terragrunt.values.hcl"), "/middle")
				assert.Contains(t, stdout, "consumer = true")
				assert.NotContains(t, stdout, `relay_marker = "old"`)
				assert.NotContains(t, stdout, `relay_marker = "obsolete"`)

				switch {
				case includeExternal:
					assert.Equal(t, 3, strings.Count(stdout, `relay_marker = "new"`))
				case insideGeneratedTree:
					assert.Equal(t, 1, strings.Count(stdout, `relay_marker = "new"`))
				default:
					assert.NotContains(t, stdout, "relay_marker")
				}
			})
		}
	}
}

func TestTFDependencyStackDiscoveryReportsUnrepairedUnitErrors(t *testing.T) {
	t.Parallel()
	f := newDependencyStackFixture(t, true)
	f.write("entry/broken/terragrunt.hcl", `locals {
 marker = missing.value
 other = absent.value
}`)
	f.write(
		"entry/consumer/terragrunt.hcl",
		fmt.Sprintf(`dependencies { paths = [%q] }`, filepath.ToSlash(filepath.Join(f.root, "producer"))),
	)
	f.producerSource("new")

	stdout, stderr, err := f.run("entry", "run --all --queue-include-external -- plan")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "Could not evaluate all locals")
	assert.Contains(t, stderr, "missing")
	assert.Contains(t, stderr, "absent")
	assert.Contains(t, stderr, "level=error")
	assert.NotContains(t, stdout, "Changes to Outputs")
	assert.Contains(t, f.read("producer/.terragrunt-stack/db/terragrunt.hcl"), `marker = "new"`)
}

func TestTFDependencyStackDiscoveryReportsGenerationErrorsImmediately(t *testing.T) {
	t.Parallel()
	f := newDependencyStackFixture(t, true)
	f.write(".sources/db/terragrunt.hcl", `dependencies { paths = [missing.value] }`)
	f.write(
		"entry/terragrunt.hcl",
		fmt.Sprintf(`dependencies { paths = [%q] }`, filepath.ToSlash(filepath.Join(f.root, "producer"))),
	)

	stdout, stderr, err := f.run("entry", "run --all --queue-include-external -- plan")

	require.Error(t, err)
	assert.Contains(t, stderr, "missing")
	assert.Contains(t, stderr, "level=error")
	assert.Equal(t, 1, strings.Count(stderr, "Generating unit db from "))
	assert.NotContains(t, stdout, "Changes to Outputs")
}
