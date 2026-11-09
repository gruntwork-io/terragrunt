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
)

func TestTFDependencyStackGenerateRecursesWithoutState(t *testing.T) {
	t.Parallel()
	for _, shape := range []string{"transitive", "diamond", "nested", "initial traversal"} {
		t.Run(shape, func(t *testing.T) {
			t.Parallel()
			f := newDependencyStackFixture(t, true)
			// Generation must discover dependencies without evaluating their outputs.
			f.write(".sources/db/terragrunt.hcl", "inputs = { marker = \"new\" }")
			f.write("middle/terragrunt.stack.hcl", fmt.Sprintf("unit \"middle\" {\n source = %q\n path = \"middle\"\n}\n", filepath.Join(f.root, ".sources/middle")))
			f.write(".sources/middle/terragrunt.hcl", fmt.Sprintf("dependency \"producer\" {\n config_path = %q\n}\ninputs = { marker = dependency.producer.outputs.db.marker }\n", filepath.Join(f.root, "producer")))
			consumer := fmt.Sprintf("unit \"app\" {\n source = %q\n path = \"app\"\n values = { producer = %q }\n}\n", filepath.Join(f.root, ".sources/app"), filepath.Join(f.root, "middle"))
			dir := "consumer"
			expected := []string{"middle/.terragrunt-stack/middle/terragrunt.hcl", "producer/.terragrunt-stack/db/terragrunt.hcl"}
			switch shape {
			case "diamond":
				consumer += fmt.Sprintf("unit \"other\" {\n source = %q\n path = \"other\"\n values = { producer = %q }\n}\n", filepath.Join(f.root, ".sources/app"), filepath.Join(f.root, "producer"))
			case "nested":
				f.write("middle/terragrunt.stack.hcl", fmt.Sprintf("stack \"nested\" {\n source = %q\n path = \"nested\"\n}\n", filepath.Join(f.root, "nested-source")))
				f.write("nested-source/terragrunt.stack.hcl", fmt.Sprintf("unit \"middle\" {\n source = %q\n path = \"middle\"\n}\n", filepath.Join(f.root, ".sources/middle")))
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
			assert.NoError(t, err, "%s", stderr)
			for _, path := range expected {
				assert.FileExists(t, filepath.Join(f.root, path))
			}
			assert.Equal(t, 1, strings.Count(stderr, "Generating unit db from "), "shared producer must be generated once per command")
			if dir != "." {
				assert.NoDirExists(t, filepath.Join(f.root, "unrelated/.terragrunt-stack"))
			}
		})
	}
}

func TestTFDependencyStackGenerateReportsCycle(t *testing.T) {
	t.Parallel()
	f := newDependencyStackFixture(t, true)
	f.write(".sources/db/terragrunt.hcl", fmt.Sprintf("dependency \"consumer\" {\n config_path = %q\n}\n", filepath.Join(f.root, "consumer")))
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	var stdout, stderr bytes.Buffer
	err := helpers.RunTerragruntCommandWithVenv(t, ctx, f.v.WithWriter(&stdout).WithErrWriter(&stderr),
		"terragrunt --non-interactive --working-dir "+filepath.Join(f.root, "consumer")+" stack generate")
	assert.Error(t, err)
	assert.NoError(t, ctx.Err(), "cycle detection must terminate before the deadline")
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
			assert.NoError(t, err, "%s", stderr)
			assert.Contains(t, stdout, `consumed = "new"`)
			assert.Equal(t, 1, strings.Count(stderr, "Generating unit db from "))
			assert.NoDirExists(t, filepath.Join(f.root, "consumer/.terragrunt-stack"))
			assert.NoDirExists(t, filepath.Join(f.root, "unrelated/.terragrunt-stack"))
		})
	}
}
