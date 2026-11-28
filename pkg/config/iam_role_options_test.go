package config_test

import (
	"context"
	"fmt"
	"maps"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/iam"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/internal/vexec"
	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// callerARNLocal is a locals block that reads the identity of the role a parse evaluates with.
const callerARNLocal = `
locals {
  arn = get_aws_caller_identity_arn()
}
`

// TestResolveRunConfigAssumesTheRoleOfIdenticalContentInEachDirectory pins that two configs with the same content
// each evaluate with their own IAM role when the role depends on the directory the config is in.
func TestResolveRunConfigAssumesTheRoleOfIdenticalContentInEachDirectory(t *testing.T) {
	t.Parallel()

	const content = `
iam_role               = "arn:aws:iam::123456789012:role/${basename(get_terragrunt_dir())}"
iam_web_identity_token = "fake-token"
` + callerARNLocal

	root := venvtest.Root("/iam-identical-content")
	sts, _ := memSTS()
	v := venv.MemVenv().WithHTTP(sts).WithFS(venvtest.NewFS(t, root, map[string]string{
		filepath.Join("a", config.DefaultTerragruntConfigPath): content,
		filepath.Join("b", config.DefaultTerragruntConfigPath): content,
	}))
	s := new(config.Store)

	for _, name := range []string{"a", "b"} {
		cfgPath := filepath.Join(root, name, config.DefaultTerragruntConfigPath)
		ctx, pctx := newIAMParsingContext(t, cfgPath)

		cfg, err := s.ResolveRunConfig(ctx, logger.CreateLogger(), v, pctx, cfgPath)
		require.NoError(t, err)
		assert.Equal(t, "arn:aws:iam::123456789012:role/"+name, cfg.Locals["arn"])
	}
}

// TestResolveRunConfigAssumesTheRoleOfAnInclude pins that a config evaluates with the role a file it merges sets,
// including when the include path reads the config's values.
func TestResolveRunConfigAssumesTheRoleOfAnInclude(t *testing.T) {
	t.Parallel()

	const role = "arn:aws:iam::123456789012:role/root"

	testCases := []struct {
		files   map[string]string
		name    string
		include string
	}{
		{name: "default merge", include: `path = find_in_parent_folders("root.hcl")`},
		{
			name:    "deep merge",
			include: `path = find_in_parent_folders("root.hcl")` + "\n" + `merge_strategy = "deep"`,
		},
		{
			name:    "path from values",
			include: `path = "${values.root_dir}/root.hcl"`,
			files:   map[string]string{filepath.Join("unit", "terragrunt.values.hcl"): `root_dir = ".."`},
		},
	}

	for i, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			root := venvtest.Root(fmt.Sprintf("/iam-include-%d", i))
			cfgPath := filepath.Join(root, "unit", config.DefaultTerragruntConfigPath)

			files := map[string]string{
				"root.hcl": `iam_role = "` + role + `"` + "\n",
				filepath.Join("unit", config.DefaultTerragruntConfigPath): "include \"root\" {\n" + tc.include + "\n}\n" +
					callerARNLocal,
			}
			maps.Copy(files, tc.files)

			sts, _ := memSTS()
			v := venv.MemVenv().WithHTTP(sts).WithFS(venvtest.NewFS(t, root, files))
			ctx, pctx := newIAMParsingContext(t, cfgPath)

			cfg, err := new(config.Store).ResolveRunConfig(
				ctx,
				logger.CreateLogger(),
				v,
				pctx,
				cfgPath,
			)
			require.NoError(t, err)
			assert.Equal(t, role, cfg.Locals["arn"])
		})
	}
}

// TestResolveRunConfigRunsTheCommandOfAnIncludePathOncePerPass pins that a config whose include path calls run_cmd
// runs the command twice: once resolving the role the include sets, and once in the parse that evaluates with it.
func TestResolveRunConfigRunsTheCommandOfAnIncludePathOncePerPass(t *testing.T) {
	t.Parallel()

	const role = "arn:aws:iam::123456789012:role/root"

	root := venvtest.Root("/iam-include-run-cmd")
	cfgPath := filepath.Join(root, "unit", config.DefaultTerragruntConfigPath)

	var runs atomic.Int64

	exec := vexec.NewMemExec(func(_ context.Context, inv vexec.Invocation) vexec.Result {
		runs.Add(1)

		return vexec.Result{Stdout: []byte(inv.Args[len(inv.Args)-1])}
	}, vexec.WithLookPath(func(file string) (string, error) { return "/usr/bin/" + file, nil }))

	sts, _ := memSTS()
	v := venv.MemVenv().WithExec(exec).WithHTTP(sts).WithFS(venvtest.NewFS(t, root, map[string]string{
		"root.hcl": `iam_role = "` + role + `"` + "\n",
		filepath.Join("unit", config.DefaultTerragruntConfigPath): `
include "root" {
  path = run_cmd("--terragrunt-quiet", "--terragrunt-no-cache", "echo", "../root.hcl")
}
` + callerARNLocal,
	}))

	ctx, pctx := newIAMParsingContext(t, cfgPath)

	cfg, err := new(config.Store).ResolveRunConfig(
		ctx,
		logger.CreateLogger(),
		v,
		pctx,
		cfgPath,
	)
	require.NoError(t, err)
	assert.Equal(t, role, cfg.Locals["arn"])
	assert.Equal(t, int64(2), runs.Load())
}

// newIAMParsingContext returns a context for parsing cfgPath that assumes roles with a web identity token. The run
// evaluates with a role of its own until the config's role resolves.
func newIAMParsingContext(t *testing.T, cfgPath string) (context.Context, *config.ParsingContext) {
	t.Helper()

	ctx, pctx := newTestParsingContext(t, cfgPath)
	pctx.IAMRoleOptions = iam.RoleOptions{
		RoleARN:          "arn:aws:iam::111111111111:role/run",
		WebIdentityToken: "fake-token",
	}
	pctx.OriginalIAMRoleOptions = iam.RoleOptions{WebIdentityToken: "fake-token"}

	return ctx, pctx
}
