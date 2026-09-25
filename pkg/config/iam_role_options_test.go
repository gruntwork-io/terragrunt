package config_test

import (
	"os"
	"path/filepath"
	"testing"

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
