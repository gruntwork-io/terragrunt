package config_test

import (
	"context"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/internal/vexec"
	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/pkg/config/hclparse"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestResolveIAMRoleOptionsIdenticalContentInTwoDirectories pins that two
// configs with the same content resolve their own IAM role when the role
// depends on the directory the config is in.
func TestResolveIAMRoleOptionsIdenticalContentInTwoDirectories(t *testing.T) {
	t.Parallel()

	const content = `iam_role = "arn:aws:iam::123456789012:role/${basename(get_terragrunt_dir())}"` + "\n"

	tmpDir := t.TempDir()

	for _, name := range []string{"a", "b"} {
		dir := filepath.Join(tmpDir, name)
		require.NoError(t, os.MkdirAll(dir, 0755))

		cfgPath := filepath.Join(dir, config.DefaultTerragruntConfigPath)
		require.NoError(t, os.WriteFile(cfgPath, []byte(content), 0644))

		file, err := hclparse.NewParser().ParseFromString(content, cfgPath)
		require.NoError(t, err)

		v := venvtest.NewWithOSFS()
		ctx, pctx := newTestParsingContext(t, cfgPath)

		opts, err := config.ResolveIAMRoleOptions(ctx, logger.CreateLogger(), v, new(config.Store), pctx, file, nil)
		require.NoError(t, err)
		assert.Equal(t, "arn:aws:iam::123456789012:role/"+name, opts.RoleARN)
	}
}

// TestResolveIAMRoleOptionsReadsTheRoleOfAnInclude pins that the role an included file sets reaches the including
// config unless the include does not merge, including when the include path reads the config's values.
func TestResolveIAMRoleOptionsReadsTheRoleOfAnInclude(t *testing.T) {
	t.Parallel()

	const role = "arn:aws:iam::123456789012:role/root"

	testCases := []struct {
		files   map[string]string
		name    string
		include string
		want    string
	}{
		{name: "default merge", include: `path = find_in_parent_folders("root.hcl")`, want: role},
		{
			name:    "deep merge",
			include: `path = find_in_parent_folders("root.hcl")` + "\n" + `merge_strategy = "deep"`,
			want:    role,
		},
		{name: "no merge", include: `path = find_in_parent_folders("root.hcl")` + "\n" + `merge_strategy = "no_merge"`},
		{
			name:    "path from values",
			include: `path = "${values.root_dir}/root.hcl"`,
			files:   map[string]string{filepath.Join("unit", "terragrunt.values.hcl"): `root_dir = ".."`},
			want:    role,
		},
	}

	for i, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			root := venvtest.Root(fmt.Sprintf("/iam-include-%d", i))
			unit := "include \"root\" {\n" + tc.include + "\n}\n"
			cfgPath := filepath.Join(root, "unit", config.DefaultTerragruntConfigPath)

			files := map[string]string{
				"root.hcl": `iam_role = "` + role + `"` + "\n",
				filepath.Join("unit", config.DefaultTerragruntConfigPath): unit,
			}
			maps.Copy(files, tc.files)

			v := venv.MemVenv().WithFS(venvtest.NewFS(t, root, files))

			file, err := hclparse.NewParser().ParseFromString(unit, cfgPath)
			require.NoError(t, err)

			ctx, pctx := newTestParsingContext(t, cfgPath)

			opts, err := config.ResolveIAMRoleOptions(ctx, logger.CreateLogger(), v, new(config.Store), pctx, file, nil)
			require.NoError(t, err)
			assert.Equal(t, tc.want, opts.RoleARN)
		})
	}
}

// TestResolveIAMRoleOptionsRunsTheCommandOfAnIncludePathOnce pins that resolving the role of a config whose include
// path calls run_cmd runs the command once: in the partial parse of the Terragrunt flags.
func TestResolveIAMRoleOptionsRunsTheCommandOfAnIncludePathOnce(t *testing.T) {
	t.Parallel()

	root := venvtest.Root("/iam-include-run-cmd")
	unit := `
include "root" {
  path = run_cmd("--terragrunt-quiet", "--terragrunt-no-cache", "echo", "../root.hcl")
}
`
	cfgPath := filepath.Join(root, "unit", config.DefaultTerragruntConfigPath)

	var runs atomic.Int64

	exec := vexec.NewMemExec(func(_ context.Context, inv vexec.Invocation) vexec.Result {
		runs.Add(1)

		return vexec.Result{Stdout: []byte(inv.Args[len(inv.Args)-1])}
	}, vexec.WithLookPath(func(file string) (string, error) { return "/usr/bin/" + file, nil }))

	v := venv.MemVenv().WithExec(exec).WithFS(venvtest.NewFS(t, root, map[string]string{
		"root.hcl": `iam_role = "arn:aws:iam::123456789012:role/root"` + "\n",
		filepath.Join("unit", config.DefaultTerragruntConfigPath): unit,
	}))

	file, err := hclparse.NewParser().ParseFromString(unit, cfgPath)
	require.NoError(t, err)

	ctx, pctx := newTestParsingContext(t, cfgPath)

	opts, err := config.ResolveIAMRoleOptions(ctx, logger.CreateLogger(), v, new(config.Store), pctx, file, nil)
	require.NoError(t, err)
	assert.Equal(t, "arn:aws:iam::123456789012:role/root", opts.RoleARN)
	assert.Equal(t, int64(1), runs.Load())
}
