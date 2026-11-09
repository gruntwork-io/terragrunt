//go:build tf

package test_test

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTFDependencyStackConfigReadersRejectFailedGeneration(t *testing.T) {
	t.Parallel()

	for _, entry := range []struct{ dir, command string }{
		{"consumer/.terragrunt-stack/app", "run -- plan"},
		{"consumer/.terragrunt-stack/app", "render --json"},
		{"consumer/.terragrunt-stack/app", "exec -- sh -c 'echo should-not-run'"},
		{"producer", "stack output --format json"},
	} {
		t.Run(entry.command, func(t *testing.T) {
			t.Parallel()
			f := newDependencyStackFixture(t, true)
			f.generate("consumer")
			f.generate("producer")
			f.write("producer/terragrunt.stack.hcl", "unit \"db\" {\n source = \"../missing\"\n path = \"db\"\n}\n")
			before := f.read("states/old.tfstate")
			stdout, stderr, err := f.run(entry.dir, entry.command)
			require.Error(t, err, "%s", stderr)
			assert.Contains(t, err.Error(), "missing")
			assert.NotContains(t, stdout, "should-not-run")
			assert.Equal(t, before, f.read("states/old.tfstate"))
		})
	}
}

func TestTFDependencyStackExecHonorsOptOut(t *testing.T) {
	t.Parallel()

	for _, environment := range []bool{false, true} {
		t.Run(fmt.Sprintf("environment=%t", environment), func(t *testing.T) {
			t.Parallel()
			f := newDependencyStackFixture(t, true)
			f.generate("consumer")
			f.generate("producer")
			f.producerSource("new")

			flag := "--no-stack-generate"

			if environment {
				f.v.Env["TG_NO_STACK_GENERATE"] = "true"
				flag = ""
			}

			stdout, stderr, err := f.run(
				"consumer/.terragrunt-stack/app",
				"exec "+flag+" -- sh -c 'printf %s \"$TF_VAR_marker\"'",
			)
			require.NoError(t, err, "%s", stderr)
			assert.Contains(t, stdout, "old")
			assert.Contains(t, f.read("producer/.terragrunt-stack/db/terragrunt.hcl"), `marker = "old"`)
		})
	}
}

func TestTFDependencyStackRenderAllSharesGeneration(t *testing.T) {
	t.Parallel()
	f := newDependencyStackFixture(t, true)
	f.generate("consumer")
	f.generate("producer")
	f.producerSource("new")
	f.write(
		"consumer/other/terragrunt.hcl",
		fmt.Sprintf(
			"dependency \"producer\" {\n config_path = %q\n}\ninputs = { marker = dependency.producer.outputs.db.marker }\n",
			filepath.Join(f.root, "producer"),
		),
	)
	stdout, stderr, err := f.run("consumer", "render --all --json --source-update")
	require.NoError(t, err, "%s", stderr)
	assert.Equal(t, 2, strings.Count(stdout, `"marker":"new"`))
	assert.Equal(t, 1, strings.Count(stderr, "Generating unit db from "))
}

func TestTFDependencyStackBackendReadersHonorOptOut(t *testing.T) {
	t.Parallel()

	for _, command := range []string{"bootstrap", "delete"} {
		for _, optOut := range []string{"", "--no-stack-generate", "environment"} {
			t.Run(command+"/"+optOut, func(t *testing.T) {
				t.Parallel()
				f := newDependencyStackFixture(t, true)
				f.generate("producer")
				f.producerSource("new")
				// A string where the backend requires a map stops before backend operations.
				f.write("backend/terragrunt.hcl", fmt.Sprintf(`dependency "producer" {
 config_path = %q
}
remote_state {
 backend = "local"
 config = dependency.producer.outputs.db.marker
}
`, filepath.Join(f.root, "producer")))

				marker, flag := "new", optOut
				if optOut != "" {
					marker = "old"
				}

				if optOut == "environment" {
					f.v.Env["TG_NO_STACK_GENERATE"] = "true"
					flag = ""
				}

				_, _, err := f.run("backend", "backend "+command+" "+flag)
				require.Error(t, err)
				assert.Contains(t, err.Error(), "cannot unmarshal string")
				assert.Contains(t, f.read("producer/.terragrunt-stack/db/terragrunt.hcl"), `marker = "`+marker+`"`)
			})
		}
	}
}
