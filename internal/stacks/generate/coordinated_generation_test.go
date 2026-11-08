package generate_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	rewrite "github.com/gruntwork-io/terragrunt/internal/config/rewrite"
	"github.com/gruntwork-io/terragrunt/internal/experiment"
	"github.com/gruntwork-io/terragrunt/internal/stacks/generate"
	"github.com/gruntwork-io/terragrunt/internal/vexec"
	"github.com/gruntwork-io/terragrunt/pkg/options"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"
	"github.com/stretchr/testify/require"
)

func TestGenerateStacksSharesCompletionWithinCommand(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		t.Run(fmt.Sprintf("experiment=%t", enabled), func(t *testing.T) {
			root := t.TempDir()
			live, source := filepath.Join(root, "live"), filepath.Join(root, "source")
			require.NoError(t, os.MkdirAll(live, 0755))
			require.NoError(t, os.MkdirAll(source, 0755))
			sourceFile := filepath.Join(source, "terragrunt.hcl")
			require.NoError(t, os.WriteFile(sourceFile, []byte("# old"), 0600))
			require.NoError(t, os.WriteFile(filepath.Join(live, "terragrunt.stack.hcl"), []byte(fmt.Sprintf("unit \"app\" {\n source = %q\n path = \"app\"\n}\n", source)), 0600))
			opts := options.NewTerragruntOptions(vexec.NewOSExec())
			opts.WorkingDir, opts.RootWorkingDir = live, live
			opts.Parallelism, opts.NoCAS, opts.SourceUpdate = 1, true, true
			if enabled {
				require.NoError(t, opts.Experiments.EnableExperiment(experiment.DependencyStackGeneration))
			}
			v := venvtest.NewOSWithEmptyEnv()
			s := new(rewrite.Store)
			ctx := generate.WithCoordinator(t.Context(), v.FS, opts)
			run := func(ctx context.Context) {
				require.NoError(t, generate.NewGenerator().GenerateStacks(ctx, logger.CreateLogger(), v, s, opts, nil))
			}
			run(ctx)
			output := filepath.Join(live, ".terragrunt-stack", "app", "terragrunt.hcl")
			data, err := os.ReadFile(output)
			require.NoError(t, err)
			require.Equal(t, "# old", string(data))
			require.NoError(t, os.WriteFile(sourceFile, []byte("# new"), 0600))
			run(ctx)
			data, err = os.ReadFile(output)
			require.NoError(t, err)
			if enabled {
				require.Equal(t, "# old", string(data))
			} else {
				require.Equal(t, "# new", string(data))
			}
			run(generate.WithCoordinator(t.Context(), v.FS, opts))
			data, err = os.ReadFile(output)
			require.NoError(t, err)
			require.Equal(t, "# new", string(data))
		})
	}
}

func TestGenerateStacksCachesWorkerFailureUntilNextCommand(t *testing.T) {
	root := t.TempDir()
	live, source := filepath.Join(root, "live"), filepath.Join(root, "source")
	require.NoError(t, os.MkdirAll(live, 0755))
	require.NoError(t, os.MkdirAll(source, 0755))
	// A downloaded unit without terragrunt.hcl fails worker-side validation.
	require.NoError(t, os.WriteFile(filepath.Join(source, "main.tf"), nil, 0600))
	require.NoError(t, os.WriteFile(filepath.Join(live, "terragrunt.stack.hcl"), []byte(fmt.Sprintf("unit \"app\" {\n source = %q\n path = \"app\"\n}\n", source)), 0600))
	opts := options.NewTerragruntOptions(vexec.NewOSExec())
	opts.WorkingDir, opts.RootWorkingDir = live, live
	opts.Parallelism, opts.NoCAS, opts.SourceUpdate = 1, true, true
	require.NoError(t, opts.Experiments.EnableExperiment(experiment.DependencyStackGeneration))
	v := venvtest.NewOSWithEmptyEnv()
	s := new(rewrite.Store)
	ctx := generate.WithCoordinator(t.Context(), v.FS, opts)
	run := func(ctx context.Context) error {
		return generate.NewGenerator().GenerateStacks(ctx, logger.CreateLogger(), v, s, opts, nil)
	}
	first := run(ctx)
	require.Error(t, first)
	require.NoError(t, os.WriteFile(filepath.Join(source, "terragrunt.hcl"), nil, 0600))
	require.EqualError(t, run(ctx), first.Error())
	require.NoError(t, run(generate.WithCoordinator(t.Context(), v.FS, opts)))
}

func TestGenerateStacksRefreshesNestedStacksParentFirst(t *testing.T) {
	root := t.TempDir()
	live, child, unit := filepath.Join(root, "live"), filepath.Join(root, "child"), filepath.Join(root, "unit")
	for _, dir := range []string{live, child, unit} {
		require.NoError(t, os.MkdirAll(dir, 0755))
	}
	require.NoError(t, os.WriteFile(filepath.Join(live, "terragrunt.stack.hcl"), []byte(fmt.Sprintf("stack \"child\" {\n source = %q\n path = \"child\"\n}\n", child)), 0600))
	require.NoError(t, os.WriteFile(filepath.Join(child, "terragrunt.stack.hcl"), []byte(fmt.Sprintf("unit \"app\" {\n source = %q\n path = \"app\"\n}\n", unit)), 0600))
	sourceFile := filepath.Join(unit, "terragrunt.hcl")
	require.NoError(t, os.WriteFile(sourceFile, []byte("# old"), 0600))
	opts := options.NewTerragruntOptions(vexec.NewOSExec())
	opts.WorkingDir, opts.RootWorkingDir = live, live
	opts.Parallelism, opts.NoCAS, opts.SourceUpdate = 2, true, true
	require.NoError(t, opts.Experiments.EnableExperiment(experiment.DependencyStackGeneration))
	v := venvtest.NewOSWithEmptyEnv()
	s := new(rewrite.Store)
	run := func(ctx context.Context) {
		require.NoError(t, generate.NewGenerator().GenerateStacks(ctx, logger.CreateLogger(), v, s, opts, nil))
	}
	run(generate.WithCoordinator(t.Context(), v.FS, opts))
	require.NoError(t, os.WriteFile(sourceFile, []byte("# new"), 0600))
	ctx := generate.WithCoordinator(t.Context(), v.FS, opts)
	run(ctx)
	output := filepath.Join(live, ".terragrunt-stack", "child", ".terragrunt-stack", "app", "terragrunt.hcl")
	data, err := os.ReadFile(output)
	require.NoError(t, err)
	require.Equal(t, "# new", string(data))
	require.NoError(t, os.WriteFile(sourceFile, []byte("# later"), 0600))
	run(ctx)
	data, err = os.ReadFile(output)
	require.NoError(t, err)
	require.Equal(t, "# new", string(data))
}
