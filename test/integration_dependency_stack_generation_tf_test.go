//go:build tf

package test_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTFRunAllGeneratesDependencyStack(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name           string
		preGenerate    bool
		flags          string
		expectedMarker string
		expectClean    bool
	}{
		{name: "missing producer", expectedMarker: "new"},
		{name: "stale producer", preGenerate: true, expectedMarker: "new"},
		{name: "source update cleans stale producer", preGenerate: true, flags: "--source-update", expectedMarker: "new", expectClean: true},
		{name: "generation disabled preserves producer", preGenerate: true, flags: "--no-stack-generate", expectedMarker: "old"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			root := helpers.TmpDirWOSymlinks(t)
			for path, contents := range map[string]string{
				"producer/terragrunt.stack.hcl": `unit "db" {
 source = "../units/db"
 path = "db"
}
`,
				"consumer/terragrunt.stack.hcl": `unit "app" {
 source = "../units/app"
 path = "app"
 values = { producer_path = "${get_terragrunt_dir()}/../producer" }
}
`,
				"units/db/terragrunt.hcl": `terraform { source = "." }
inputs = { marker = "old" }
`,
				"units/db/main.tf": `variable "marker" { type = string }
output "marker" { value = var.marker }
`,
				"units/app/terragrunt.hcl": `terraform { source = "." }
dependency "producer" {
 config_path = values.producer_path
 mock_outputs = { db = { marker = "mock" } }
 mock_outputs_allowed_terraform_commands = ["plan"]
}
inputs = { marker = dependency.producer.outputs.db.marker }
`,
				"units/app/main.tf": `variable "marker" { type = string }
output "consumed" { value = var.marker }
`,
			} {
				target := filepath.Join(root, filepath.FromSlash(path))
				require.NoError(t, os.MkdirAll(filepath.Dir(target), 0755))
				require.NoError(t, os.WriteFile(target, []byte(contents), 0600))
			}
			producer := filepath.Join(root, "producer")
			consumer := filepath.Join(root, "consumer")
			generated := filepath.Join(producer, ".terragrunt-stack", "db")
			obsolete := filepath.Join(generated, "obsolete.txt")
			if tc.preGenerate {
				for _, dir := range []string{producer, consumer} {
					_, stderr, err := helpers.RunTerragruntCommandWithOutput(t,
						"terragrunt stack generate --no-cas --working-dir "+dir)
					require.NoError(t, err, "%s", stderr)
				}
				require.NoError(t, os.WriteFile(obsolete, []byte("stale"), 0600))
			}
			require.NoError(t, os.WriteFile(filepath.Join(root, "units", "db", "terragrunt.hcl"),
				[]byte("terraform { source = \".\" }\ninputs = { marker = \"new\" }\n"), 0600))

			stdout, stderr, err := helpers.RunTerragruntCommandWithOutput(t,
				"terragrunt run --experiment dependency-stack-generation --all --non-interactive --no-cas --queue-include-external "+tc.flags+
					" --working-dir "+consumer+" -- plan")
			assert.NoError(t, err, "%s", stderr)
			assert.Contains(t, stdout, `consumed = "mock"`)
			assert.Contains(t, stdout, `marker = "`+tc.expectedMarker+`"`)
			require.FileExists(t, filepath.Join(generated, "terragrunt.hcl"))

			rendered, renderStderr, renderErr := helpers.RunTerragruntCommandWithOutput(t,
				"terragrunt render --json --no-cas --working-dir "+generated)
			require.NoError(t, renderErr, "%s", renderStderr)
			var cfg struct {
				Inputs struct {
					Marker string `json:"marker"`
				} `json:"inputs"`
			}
			require.NoError(t, json.Unmarshal([]byte(rendered), &cfg))
			assert.Equal(t, tc.expectedMarker, cfg.Inputs.Marker)
			if tc.expectClean {
				assert.NoFileExists(t, obsolete)
			} else if tc.preGenerate {
				assert.FileExists(t, obsolete)
			}
		})
	}
}
