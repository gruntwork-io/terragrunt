package cli_test

import (
	"context"
	"io"
	"os/exec"
	"path/filepath"
	"slices"
	"sync"
	"testing"

	"github.com/hashicorp/hcl/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gruntwork-io/terragrunt/internal/cli"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/internal/vexec"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/pkg/options"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"
)

// TestVersionAttributeReferencesDependencyIsRejectedWithRacing pins that `run` and `run --all` both reject a unit
// whose terraform_version_constraint references a dependency output, even in a branch that never evaluates, before
// they run any OpenTofu command beyond a version probe.
func TestVersionAttributeReferencesDependencyIsRejectedWithRacing(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		workingDir string
		args       []string
	}{
		{name: "run", workingDir: "unit", args: []string{"run"}},
		{name: "run --all", workingDir: ".", args: []string{"run", "--all"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var (
				mu       sync.Mutex
				commands []tfCommand
			)

			memExec := vexec.NewMemExec(
				func(_ context.Context, inv vexec.Invocation) vexec.Result {
					mu.Lock()
					defer mu.Unlock()

					commands = append(commands, tfCommand{name: inv.Name, dir: inv.Dir, args: slices.Clone(inv.Args)})

					return vexec.Result{Stdout: []byte("OpenTofu v1.9.0\n")}
				},
				vexec.WithLookPath(func(file string) (string, error) {
					if file != tofuBinary {
						return "", exec.ErrNotFound
					}

					return filepath.Join(mockBinDir, file), nil
				}),
			)

			root := venvtest.Root("/live")
			v := venv.MemVenv().WithExec(memExec).WithFS(venvtest.NewFS(t, root, map[string]string{
				"dep/terragrunt.hcl":  "",
				"dep/main.tf":         "",
				"unit/main.tf":        "",
				"unit/terragrunt.hcl": versionAttributeDependencyUnit,
			}))

			l := logger.CreateLogger()
			l.SetOptions(log.WithOutput(io.Discard))

			args := append([]string{"terragrunt"}, tt.args...)
			args = append(args, "--non-interactive", "--working-dir", filepath.Join(root, tt.workingDir), "--", "plan")

			err := cli.NewApp(l, options.NewTerragruntOptions(), v).RunContext(t.Context(), l, v, args)

			var diags hcl.Diagnostics
			require.ErrorAs(t, err, &diags)
			require.Len(t, diags, 1)

			assert.Equal(t, "Dependency reference not allowed", diags[0].Summary)
			require.NotNil(t, diags[0].Subject)
			assert.Equal(t, filepath.Join(root, "unit", "terragrunt.hcl"), diags[0].Subject.Filename)

			mu.Lock()
			defer mu.Unlock()

			for _, cmd := range commands {
				assert.True(t, isTFVersionProbe(cmd), "ran %s %v", cmd.name, cmd.args)
			}
		})
	}
}

const versionAttributeDependencyUnit = `
locals {
  use_dependency = false
}

dependency "dep" {
  config_path = "../dep"

  mock_outputs = {
    constraint = ">= 1.0"
  }
}

terraform_version_constraint = local.use_dependency ? dependency.dep.outputs.constraint : ">= 1.0"
`
