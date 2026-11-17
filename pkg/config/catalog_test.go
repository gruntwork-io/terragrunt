package config_test

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/vexec"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCatalogParseConfigFile(t *testing.T) {
	t.Parallel()

	basePath, err := filepath.Abs(filepath.Join("..", "..", "test", "fixtures", "catalog"))
	require.NoError(t, err)

	testCases := []struct {
		expectedErr    error
		expectedConfig *config.CatalogConfig
		configPath     string
	}{
		{
			configPath: filepath.Join(basePath, "config1.hcl"),
			expectedConfig: &config.CatalogConfig{
				URLs: []string{
					filepath.Join(
						basePath,
						"terraform-aws-eks",
					), // this path exists in the fixture directory and must be converted to the absolute path.
					"/repo-copier",
					"./terraform-aws-service-catalog",
					"/project/terragrunt/test/terraform-aws-vpc",
					"github.com/gruntwork-io/terraform-aws-lambda",
				},
			},
		},
		{
			configPath: filepath.Join(basePath, "config2.hcl"),
		},
		{
			configPath:     filepath.Join(basePath, "config3.hcl"),
			expectedConfig: &config.CatalogConfig{},
		},
		{
			configPath: filepath.Join(basePath, "complex-legacy-root/terragrunt.hcl"),
			expectedConfig: &config.CatalogConfig{
				URLs: []string{
					filepath.Join(
						basePath,
						"complex-legacy-root/dev/us-west-1/modules/terraform-aws-eks",
					),
					"./terraform-aws-service-catalog",
					"https://github.com/gruntwork-io/terraform-aws-utilities",
				},
			},
		},
		{
			configPath: filepath.Join(basePath, "complex/root.hcl"),
			expectedConfig: &config.CatalogConfig{
				URLs: []string{
					filepath.Join(basePath, "complex/dev/us-west-1/modules/terraform-aws-eks"),
					"./terraform-aws-service-catalog",
					"https://github.com/gruntwork-io/terraform-aws-utilities",
				},
			},
		},
		{
			configPath: filepath.Join(basePath, "complex-legacy-root/dev/terragrunt.hcl"),
			expectedConfig: &config.CatalogConfig{
				URLs: []string{
					filepath.Join(
						basePath,
						"complex-legacy-root/dev/us-west-1/modules/terraform-aws-eks",
					),
					"./terraform-aws-service-catalog",
					"https://github.com/gruntwork-io/terraform-aws-utilities",
				},
			},
		},
		{
			configPath: filepath.Join(basePath, "complex/dev/root.hcl"),
			expectedConfig: &config.CatalogConfig{
				URLs: []string{
					filepath.Join(basePath, "complex/dev/us-west-1/modules/terraform-aws-eks"),
					"./terraform-aws-service-catalog",
					"https://github.com/gruntwork-io/terraform-aws-utilities",
				},
			},
		},
		{
			configPath: filepath.Join(basePath, "complex/dev/us-west-1/terragrunt.hcl"),
			expectedConfig: &config.CatalogConfig{
				URLs: []string{
					filepath.Join(basePath, "complex/dev/us-west-1/modules/terraform-aws-eks"),
					"./terraform-aws-service-catalog",
					"https://github.com/gruntwork-io/terraform-aws-utilities",
				},
			},
		},
		{
			configPath: filepath.Join(basePath, "complex/dev/us-west-1/modules/terragrunt.hcl"),
			expectedConfig: &config.CatalogConfig{
				URLs: []string{
					filepath.Join(basePath, "complex/dev/us-west-1/modules/terraform-aws-eks"),
					"./terraform-aws-service-catalog",
					"https://github.com/gruntwork-io/terraform-aws-utilities",
				},
			},
		},
		{
			configPath: filepath.Join(basePath, "complex/prod/terragrunt.hcl"),
			expectedConfig: &config.CatalogConfig{
				URLs: []string{
					filepath.Join(basePath, "complex/dev/us-west-1/modules/terraform-aws-eks"),
					"./terraform-aws-service-catalog",
					"https://github.com/gruntwork-io/terraform-aws-utilities",
				},
			},
		},
		{
			configPath: filepath.Join(basePath, "config4.hcl"),
			expectedConfig: &config.CatalogConfig{
				DefaultTemplate: "/test/fixtures/scaffold/external-template",
			},
		},
	}

	for i, tt := range testCases {
		t.Run(fmt.Sprintf("testCase-%d", i), func(t *testing.T) {
			t.Parallel()

			l := logger.CreateLogger()
			v := venvtest.NewWithOSFS()
			ctx, catalogPctx := newTestParsingContext(t, tt.configPath)
			catalogPctx.ScaffoldRootFileName = filepath.Base(tt.configPath)
			config, err := config.ReadCatalogConfig(ctx, l, v, catalogPctx)

			if tt.expectedErr == nil {
				require.NoError(t, err)
				assert.Equal(t, tt.expectedConfig, config)
			} else {
				assert.EqualError(t, err, tt.expectedErr.Error())
			}
		})
	}
}

// TestReadCatalogConfigRunsCommandsBesideTheConfigNamingThem pins where run_cmd
// runs while the catalog block is read. The block's own commands run beside the
// root config, since the child config generated for the read has no directory
// on disk. A command in a config the root reads runs beside that config.
func TestReadCatalogConfigRunsCommandsBesideTheConfigNamingThem(t *testing.T) {
	t.Parallel()

	for name, tc := range map[string]struct {
		files   map[string]string
		wantDir string
	}{
		"in the catalog block": {
			files: map[string]string{
				"root.hcl": `catalog { urls = [run_cmd("--terragrunt-quiet", "print-url")] }`,
			},
		},
		"in a local of the root config": {
			files: map[string]string{
				"root.hcl": `
locals { url = run_cmd("--terragrunt-quiet", "print-url") }
catalog { urls = [local.url] }
`,
			},
		},
		"in a config the root config reads": {
			files: map[string]string{
				"root.hcl": `
locals { shared = read_terragrunt_config("${get_parent_terragrunt_dir()}/shared/shared.hcl") }
catalog { urls = [local.shared.locals.url] }
`,
				"shared/shared.hcl": `locals { url = run_cmd("--terragrunt-quiet", "print-url") }`,
			},
			wantDir: "shared",
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			var (
				mu   sync.Mutex
				dirs []string
			)

			printURL := func(_ context.Context, inv vexec.Invocation) vexec.Result {
				mu.Lock()
				defer mu.Unlock()

				dirs = append(dirs, inv.Dir)

				return vexec.Result{Stdout: []byte("https://example.com/modules")}
			}

			v := venvtest.NewWithOSFS().WithExec(vexec.NewMemExec(printURL))

			rootDir, err := vfs.EvalSymlinks(v.FS, t.TempDir())
			require.NoError(t, err)

			for path, content := range tc.files {
				file := filepath.Join(rootDir, filepath.FromSlash(path))
				require.NoError(t, v.FS.MkdirAll(filepath.Dir(file), 0o755))
				require.NoError(t, vfs.WriteFile(v.FS, file, []byte(content), 0o644))
			}

			ctx, pctx := newTestParsingContext(t, filepath.Join(rootDir, "terragrunt.hcl"))
			pctx.ScaffoldRootFileName = "root.hcl"

			cfg, err := config.ReadCatalogConfig(ctx, logger.CreateLogger(), v, pctx)
			require.NoError(t, err)

			require.NotNil(t, cfg)
			assert.Equal(t, []string{"https://example.com/modules"}, cfg.URLs)

			mu.Lock()
			defer mu.Unlock()

			require.NotEmpty(t, dirs)

			for _, dir := range dirs {
				assert.Equal(t, filepath.Join(rootDir, tc.wantDir), dir)
			}
		})
	}
}

// TestParseConfigStringKeepsCommandsInAMissingConfigDir pins that only a catalog
// read moves run_cmd out of a directory that doesn't exist. Any other parse
// fails there, so a config never runs a program somewhere it did not name.
func TestParseConfigStringKeepsCommandsInAMissingConfigDir(t *testing.T) {
	t.Parallel()

	var (
		mu   sync.Mutex
		dirs []string
	)

	printDir := func(_ context.Context, inv vexec.Invocation) vexec.Result {
		mu.Lock()
		defer mu.Unlock()

		dirs = append(dirs, inv.Dir)

		return vexec.Result{}
	}

	v := venvtest.NewWithOSFS().WithExec(vexec.NewMemExec(printDir))

	configDir := filepath.Join(t.TempDir(), "missing")
	configPath := filepath.Join(configDir, "terragrunt.hcl")

	ctx, pctx := newTestParsingContext(t, configPath)

	_, err := config.ParseConfigString(ctx, logger.CreateLogger(), v, pctx, configPath,
		`locals { out = run_cmd("--terragrunt-quiet", "print-dir") }`, nil)
	require.NoError(t, err)

	mu.Lock()
	defer mu.Unlock()

	require.NotEmpty(t, dirs)

	for _, dir := range dirs {
		assert.Equal(t, configDir, dir)
	}
}
