//go:build tf

package test_test

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type dependencyStackFixture struct {
	t    *testing.T
	root string
	v    *venv.Venv
}

func newDependencyStackFixture(t *testing.T, enabled bool) *dependencyStackFixture {
	t.Helper()
	f := &dependencyStackFixture{t: t, root: helpers.TmpDirWOSymlinks(t), v: helpers.RunVenv(t)}
	f.v.Env["TG_NO_CAS"] = "true"
	f.v.Env["TG_EXPERIMENT"] = ""
	delete(f.v.Env, "TG_EXPERIMENT_MODE")
	delete(f.v.Env, "TG_SOURCE_UPDATE")
	if enabled {
		f.v.Env["TG_EXPERIMENT"] = "dependency-stack-generation"
	}
	for path, content := range map[string]string{
		"producer/terragrunt.stack.hcl": `unit "db" {
 source = "../.sources/db"
 path = "db"
}
`,
		"consumer/terragrunt.stack.hcl": `unit "app" {
 source = "../.sources/app"
 path = "app"
 values = { producer = "${get_terragrunt_dir()}/../producer" }
}
`,
		"unrelated/terragrunt.stack.hcl": `unit "unused" {
 source = "../.sources/db"
 path = "unused"
}
`,
		".sources/app/terragrunt.hcl": `terraform { source = "." }
dependency "producer" {
 config_path = values.producer
}
inputs = { marker = dependency.producer.outputs.db.marker }
`,
		".sources/app/main.tf": `variable "marker" { type = string }
output "consumed" { value = var.marker }
`,
		".sources/db/main.tf": `variable "marker" { type = string }
output "marker" { value = var.marker }
`,
	} {
		f.write(path, content)
	}
	for _, marker := range []string{"old", "new"} {
		state := fmt.Sprintf(`{"version":4,"terraform_version":"1.10.0","serial":1,"lineage":"11111111-1111-1111-1111-111111111111","outputs":{"marker":{"value":%q,"type":"string"}},"resources":[]}`, marker)
		f.write("states/"+marker+".tfstate", state)
	}
	f.producerSource("old")
	return f
}

func (f *dependencyStackFixture) write(path, content string) {
	f.t.Helper()
	target := filepath.Join(f.root, filepath.FromSlash(path))
	require.NoError(f.t, os.MkdirAll(filepath.Dir(target), 0755))
	require.NoError(f.t, os.WriteFile(target, []byte(content), 0600))
}

func (f *dependencyStackFixture) read(path string) string {
	f.t.Helper()
	data, err := os.ReadFile(filepath.Join(f.root, filepath.FromSlash(path)))
	require.NoError(f.t, err)
	return string(data)
}

func (f *dependencyStackFixture) producerSource(marker string) {
	f.t.Helper()
	f.write(".sources/db/terragrunt.hcl", fmt.Sprintf(`terraform { source = "." }
remote_state {
 backend = "local"
 generate = { path = "backend.tf", if_exists = "overwrite_terragrunt" }
 config = { path = %q }
}
inputs = { marker = %q }
`, filepath.ToSlash(filepath.Join(f.root, "states", marker+".tfstate")), marker))
}

func (f *dependencyStackFixture) run(dir, command string) (string, string, error) {
	f.t.Helper()
	command = strings.ReplaceAll(command, "{root}", filepath.ToSlash(f.root))
	return helpers.RunTerragruntCommandWithOutputWithVenv(f.t, f.v,
		"terragrunt --non-interactive --working-dir "+filepath.Join(f.root, filepath.FromSlash(dir))+" "+command)
}

func (f *dependencyStackFixture) generate(dir string) {
	f.t.Helper()
	// Fixture setup uses ordinary generation, independently of the behavior under test.
	setup := *f
	setup.v = f.v.WithEnvCloned()
	setup.v.Env["TG_EXPERIMENT"] = ""
	_, stderr, err := setup.run(dir, "stack generate")
	require.NoError(f.t, err, "%s", stderr)
}

func TestTFDependencyStackEntrypoints(t *testing.T) {
	t.Parallel()
	for _, entry := range []struct{ name, dir, command string }{
		{"generate", "consumer", "stack generate"},
		{"run all", "consumer", "run --all --queue-include-external -- plan"},
		{"single plan", "consumer/.terragrunt-stack/app", "run -- plan"},
		{"render", "consumer/.terragrunt-stack/app", "render --json"},
		{"graph plan", "consumer/.terragrunt-stack/app", "run --graph --graph-root {root}/consumer --discovery-boundary {root} --queue-include-external -- plan"},
		{"stack run", "consumer", "stack run plan"},
		{"filtered run", "consumer", "run --filter './.terragrunt-stack/app...' --queue-include-external -- plan"},
		{"single output", "consumer/.terragrunt-stack/app", "run -- output -json"},
		{"stack output", "producer", "stack output --format json"},
	} {
		for _, tc := range []struct {
			name, state, marker string
			enabled             bool
		}{
			{name: "default/missing", state: "missing"},
			{name: "default/stale", state: "stale", marker: "old"},
			{name: "default/refreshed", state: "refreshed", marker: "new"},
			{name: "enabled/missing", state: "missing", marker: "new", enabled: true},
			{name: "enabled/stale", state: "stale", marker: "new", enabled: true},
			{name: "enabled/refreshed", state: "refreshed", marker: "new", enabled: true},
		} {
			t.Run(entry.name+"/"+tc.name, func(t *testing.T) {
				t.Parallel()
				f := newDependencyStackFixture(t, tc.enabled)
				f.prepareEntrypoint(entry.name, tc.state)
				beforeOld, beforeNew := f.read("states/old.tfstate"), f.read("states/new.tfstate")

				stdout, stderr, err := f.run(entry.dir, entry.command)

				assert.Equal(t, beforeOld, f.read("states/old.tfstate"))
				assert.Equal(t, beforeNew, f.read("states/new.tfstate"))
				assert.NoDirExists(t, filepath.Join(f.root, "unrelated", ".terragrunt-stack"))
				if tc.marker == "" {
					assert.NoDirExists(t, filepath.Join(f.root, "producer", ".terragrunt-stack"))
					assertMissingDependencyStack(t, entry.name, stderr, err)
					return
				}

				require.NoError(t, err, "%s", stderr)
				path := "producer/.terragrunt-stack/db/terragrunt.hcl"
				require.FileExists(t, filepath.Join(f.root, filepath.FromSlash(path)))
				assert.Contains(t, f.read(path), `marker = "`+tc.marker+`"`)
				assertDependencyStackOutput(t, entry.name, stdout, tc.marker)
			})
		}
	}
}

func (f *dependencyStackFixture) prepareEntrypoint(entry, state string) {
	f.t.Helper()
	f.generate("consumer")
	if state != "missing" {
		f.generate("producer")
	}
	f.producerSource("new")
	if state == "refreshed" {
		f.generate("producer")
	}
	// The consumer's output remains applied state, independent of refreshed inputs.
	if entry == "single output" {
		f.write("consumer/.terragrunt-stack/app/terraform.tfstate", `{"version":4,"serial":1,"lineage":"22222222-2222-2222-2222-222222222222","outputs":{"consumed":{"value":"applied","type":"string"}},"resources":[]}`)
	}
	if entry != "graph plan" {
		return
	}
	sources := filepath.Join(helpers.TmpDirWOSymlinks(f.t), "sources")
	require.NoError(f.t, os.Rename(filepath.Join(f.root, ".sources"), sources))
	for _, stack := range []string{"consumer", "producer", "unrelated"} {
		path := stack + "/terragrunt.stack.hcl"
		f.write(path, strings.ReplaceAll(f.read(path), "../.sources", filepath.ToSlash(sources)))
	}
}

func assertMissingDependencyStack(t *testing.T, entry, stderr string, err error) {
	t.Helper()
	switch entry {
	case "generate":
		require.NoError(t, err, "%s", stderr)
	case "render":
		require.NoError(t, err, "%s", stderr)
		assert.Contains(t, stderr, "Unsupported attribute")
	default:
		require.Error(t, err)
		assert.NotContains(t, err.Error(), "not a valid")
	}
}

func assertDependencyStackOutput(t *testing.T, entry, stdout, marker string) {
	t.Helper()
	switch entry {
	case "render":
		var result struct {
			Inputs map[string]any `json:"inputs"`
		}
		require.NoError(t, json.Unmarshal([]byte(stdout), &result))
		assert.Equal(t, marker, result.Inputs["marker"])
	case "run all", "single plan", "graph plan", "stack run", "filtered run":
		assert.Contains(t, stdout, `consumed = "`+marker+`"`)
	case "stack output":
		assert.Contains(t, stdout, `"`+marker+`"`)
	case "single output":
		assert.Contains(t, stdout, `"applied"`)
	}
}

func TestTFDependencyStackInspectionPreservesFiles(t *testing.T) {
	t.Parallel()
	for _, command := range []string{"find", "list", "dag graph", "hcl validate", "info print"} {
		for _, stale := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/stale=%t", command, stale), func(t *testing.T) {
				t.Parallel()
				f := newDependencyStackFixture(t, true)
				f.generate("consumer")
				require.NoError(t, os.RemoveAll(filepath.Join(f.root, "producer", ".terragrunt-stack")))
				if stale {
					f.generate("producer")
				}
				f.producerSource("new")
				// Inspection can report an unavailable dependency, but must not materialize it.
				_, stderr, err := f.run("consumer/.terragrunt-stack/app", command)
				if stale {
					assert.NoError(t, err, "%s", stderr)
				} else if err != nil {
					assert.NotContains(t, err.Error(), "not a valid")
					assert.NotContains(t, err.Error(), "unknown command")
				}
				path := "producer/.terragrunt-stack/db/terragrunt.hcl"
				if stale {
					assert.Contains(t, f.read(path), `marker = "old"`)
				} else {
					assert.NoDirExists(t, filepath.Join(f.root, "producer", ".terragrunt-stack"))
				}
			})
		}
	}
}

func TestTFDependencyStackControls(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, flags                  string
		stale, env, disabled, direct bool
	}{
		{name: "CLI opt out missing", flags: "--no-stack-generate"},
		{name: "CLI opt out stale", flags: "--no-stack-generate", stale: true},
		{name: "environment opt out missing", env: true},
		{name: "environment opt out stale", env: true, stale: true},
		{name: "absolute discovery limit", flags: "--discovery-boundary {root}/consumer", stale: true},
		{name: "relative discovery limit", flags: "--discovery-boundary .", stale: true},
		{name: "disabled dependency", disabled: true},
		{name: "direct unit has no inferred owner", direct: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			f := newDependencyStackFixture(t, true)
			if tc.disabled {
				f.write(".sources/app/terragrunt.hcl", `dependency "producer" {
 config_path = values.producer
 enabled = false
}
inputs = { marker = "disabled" }
`)
			}
			if tc.direct {
				f.write(".sources/app/terragrunt.hcl", `dependency "producer" {
 config_path = "${values.producer}/.terragrunt-stack/db"
}
inputs = { marker = dependency.producer.outputs.marker }
`)
			}
			f.generate("consumer")
			require.NoError(t, os.RemoveAll(filepath.Join(f.root, "producer", ".terragrunt-stack")))
			if tc.stale {
				f.generate("producer")
			}
			f.producerSource("new")
			if tc.env {
				f.v.Env["TG_NO_STACK_GENERATE"] = "true"
			}
			_, stderr, err := f.run("consumer", "run --all --queue-include-external "+tc.flags+" -- plan")
			if tc.stale || tc.disabled {
				assert.NoError(t, err, "%s", stderr)
			} else {
				assert.Error(t, err)
				if err != nil {
					assert.NotContains(t, err.Error(), "not a valid")
				}
			}
			if tc.stale {
				assert.Contains(t, f.read("producer/.terragrunt-stack/db/terragrunt.hcl"), `marker = "old"`)
			} else {
				assert.NoDirExists(t, filepath.Join(f.root, "producer", ".terragrunt-stack"))
			}
		})
	}
}

func TestTFDependencyStackRejectsBadSourceDespiteStaleFiles(t *testing.T) {
	t.Parallel()
	for _, source := range []string{"../missing", "../invalid"} {
		t.Run(source, func(t *testing.T) {
			t.Parallel()
			f := newDependencyStackFixture(t, true)
			f.generate("consumer")
			require.NoError(t, os.RemoveAll(filepath.Join(f.root, "producer", ".terragrunt-stack")))
			f.generate("producer")
			f.write("invalid/terragrunt.hcl", "not valid HCL !!!")
			f.write("producer/terragrunt.stack.hcl", fmt.Sprintf("unit \"db\" {\n source = %q\n path = \"db\"\n}\n", source))
			_, _, err := f.run("consumer", "run --all --queue-include-external -- plan")
			assert.Error(t, err, "generation must reject the source instead of reusing stale files")
		})
	}
}

func TestTFDependencyStackReadTerragruntConfig(t *testing.T) {
	t.Parallel()
	f := newDependencyStackFixture(t, true)
	f.generate("consumer")
	require.NoError(t, os.RemoveAll(filepath.Join(f.root, "producer", ".terragrunt-stack")))
	f.generate("producer")
	f.write("indirect/terragrunt.hcl", fmt.Sprintf(`locals { consumer = read_terragrunt_config(%q) }
inputs = local.consumer.inputs
`, filepath.ToSlash(filepath.Join(f.root, "consumer/.terragrunt-stack/app/terragrunt.hcl"))))
	f.producerSource("new")
	stdout, stderr, err := f.run("indirect", "render --json")
	require.NoError(t, err, "%s", stderr)
	var result struct {
		Inputs map[string]any `json:"inputs"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &result))
	assert.Equal(t, "new", result.Inputs["marker"])
}

func TestTFDependencyStackSourceUpdateRespectsGenerationLimits(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, flags string
		enabled     bool
	}{
		{"default", "", false},
		{"opt out", "--no-stack-generate", true},
		{"discovery limit", "--discovery-boundary {root}/consumer", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			f := newDependencyStackFixture(t, tc.enabled)
			f.generate("consumer")
			f.generate("producer")
			f.write("producer/.terragrunt-stack/db/obsolete.txt", "preserve")
			f.producerSource("new")
			_, stderr, err := f.run("consumer", "run --all --queue-include-external --source-update "+tc.flags+" -- plan")
			require.NoError(t, err, "%s", stderr)
			assert.Contains(t, f.read("producer/.terragrunt-stack/db/terragrunt.hcl"), `marker = "old"`)
			assert.Equal(t, "preserve", f.read("producer/.terragrunt-stack/db/obsolete.txt"))
		})
	}
}

func TestTFDependencyStackOptOutAcrossEntrypoints(t *testing.T) {
	t.Parallel()
	for _, entry := range []struct{ dir, command string }{
		{"consumer", "stack generate --no-stack-generate"},
		{"consumer/.terragrunt-stack/app", "run --no-stack-generate -- plan"},
		{"producer", "stack output --no-stack-generate --format json"},
	} {
		t.Run(entry.command, func(t *testing.T) {
			t.Parallel()
			f := newDependencyStackFixture(t, true)
			f.generate("consumer")
			f.generate("producer")
			f.producerSource("new")
			_, stderr, err := f.run(entry.dir, entry.command)
			require.NoError(t, err, "%s", stderr)
			assert.Contains(t, f.read("producer/.terragrunt-stack/db/terragrunt.hcl"), `marker = "old"`)
		})
	}
}

func TestTFDependencyStackExecRefreshesInputs(t *testing.T) {
	t.Parallel()
	f := newDependencyStackFixture(t, true)
	f.generate("consumer")
	f.generate("producer")
	f.producerSource("new")
	stdout, stderr, err := f.run("consumer/.terragrunt-stack/app", "exec -- sh -c 'printf %s \"$TF_VAR_marker\"'")
	require.NoError(t, err, "%s", stderr)
	assert.Contains(t, stdout, "new")
	assert.Contains(t, f.read("producer/.terragrunt-stack/db/terragrunt.hcl"), `marker = "new"`)
}

func TestTFDependencyStackBackendDiscoveryRefreshesConfiguration(t *testing.T) {
	t.Parallel()
	f := newDependencyStackFixture(t, true)
	f.generate("consumer")
	f.generate("producer")
	f.producerSource("new")
	f.write("consumer/destination/terragrunt.hcl", "")
	// No backend is configured on the consumer: migration must stop after
	// discovery and validation, before it can pull or push state.
	_, _, err := f.run("consumer", "backend migrate .terragrunt-stack/app destination")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing remote state configuration for source module")
	assert.Contains(t, f.read("producer/.terragrunt-stack/db/terragrunt.hcl"), `marker = "new"`)
}
