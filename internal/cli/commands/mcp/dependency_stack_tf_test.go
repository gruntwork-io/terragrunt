//go:build tf

package mcp_test

import (
	"os"
	"path/filepath"
	"testing"

	tgmcp "github.com/gruntwork-io/terragrunt/internal/cli/commands/mcp"
	"github.com/gruntwork-io/terragrunt/internal/experiment"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTFPlanPreservesDependencyStacks(t *testing.T) {
	t.Parallel()
	for _, stale := range []bool{false, true} {
		name := "missing"
		if stale {
			name = "stale"
		}
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			files := map[string]string{
				"consumer/terragrunt.hcl":       "dependency \"producer\" {\n config_path = \"../producer\"\n}\n",
				"consumer/main.tf":              "output \"marker\" { value = \"consumer\" }",
				"producer/terragrunt.stack.hcl": "unit \"db\" {\n source = \"../source\"\n path = \"db\"\n}\n",
				"source/terragrunt.hcl":         "inputs = { marker = \"new\" }",
				"source/main.tf":                "output \"marker\" { value = \"new\" }",
			}
			if stale {
				files["producer/.terragrunt-stack/db/terragrunt.hcl"] = "inputs = { marker = \"old\" }"
				files["producer/.terragrunt-stack/db/main.tf"] = "output \"marker\" { value = \"old\" }"
			}
			dir := writeTree(t, files)
			session := newRealExecSession(t, dir, func(o *tgmcp.Options) {
				o.Allow = []string{"exec"}
				require.NoError(t, o.Experiments.EnableExperiment(experiment.DependencyStackGeneration))
			})
			result, err := session.CallTool(t.Context(), &mcp.CallToolParams{
				Name: "plan", Arguments: map[string]any{"working_dir": filepath.Join(dir, "consumer")},
			})
			require.NoError(t, err)
			require.NotNil(t, result)
			path := filepath.Join(dir, "producer/.terragrunt-stack/db/terragrunt.hcl")
			if stale {
				content, err := os.ReadFile(path)
				require.NoError(t, err)
				assert.Equal(t, files["producer/.terragrunt-stack/db/terragrunt.hcl"], string(content))
			} else {
				assert.NoDirExists(t, filepath.Join(dir, "producer/.terragrunt-stack"))
			}
		})
	}
}
