package config_test

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/venv"
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

		opts, err := config.ResolveIAMRoleOptions(ctx, logger.CreateLogger(), v, pctx, file, nil)
		require.NoError(t, err)
		assert.Equal(t, "arn:aws:iam::123456789012:role/"+name, opts.RoleARN)
	}
}

// TestResolveIAMRoleOptionsReadsTheRoleOfAnInclude pins that the role an included file sets reaches the including
// config unless the include does not merge.
func TestResolveIAMRoleOptionsReadsTheRoleOfAnInclude(t *testing.T) {
	t.Parallel()

	const role = "arn:aws:iam::123456789012:role/root"

	testCases := []struct {
		name     string
		strategy string
		want     string
	}{
		{name: "default merge", want: role},
		{name: "deep merge", strategy: `merge_strategy = "deep"`, want: role},
		{name: "no merge", strategy: `merge_strategy = "no_merge"`},
	}

	for i, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			root := venvtest.Root(fmt.Sprintf("/iam-include-%d", i))
			unit := `
include "root" {
  path = find_in_parent_folders("root.hcl")
  ` + tc.strategy + `
}
`
			cfgPath := filepath.Join(root, "unit", config.DefaultTerragruntConfigPath)
			v := venv.MemVenv().WithFS(venvtest.NewFS(t, root, map[string]string{
				"root.hcl": `iam_role = "` + role + `"` + "\n",
				filepath.Join("unit", config.DefaultTerragruntConfigPath): unit,
			}))

			file, err := hclparse.NewParser().ParseFromString(unit, cfgPath)
			require.NoError(t, err)

			ctx, pctx := newTestParsingContext(t, cfgPath)

			opts, err := config.ResolveIAMRoleOptions(ctx, logger.CreateLogger(), v, pctx, file, nil)
			require.NoError(t, err)
			assert.Equal(t, tc.want, opts.RoleARN)
		})
	}
}
