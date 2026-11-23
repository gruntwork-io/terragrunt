package discovery_test

import (
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/component"
	"github.com/gruntwork-io/terragrunt/internal/discovery"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/pkg/options"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestConfigFilenames pins that a filename supplied via `--config` replaces the default unit
// filenames.
func TestConfigFilenames(t *testing.T) {
	t.Parallel()

	tc := []struct {
		name       string
		configPath string
		expected   []string
		explicit   bool
	}{
		{
			name:       "resolved default path keeps the defaults",
			configPath: filepath.Join("/repo", "unit", config.DefaultTerragruntConfigPath),
			expected:   discovery.DefaultConfigFilenames,
		},
		{
			name:       "explicit terragrunt.hcl drops terragrunt.hcl.json",
			configPath: config.DefaultTerragruntConfigPath,
			explicit:   true,
			expected:   []string{config.DefaultTerragruntConfigPath, config.DefaultStackFile},
		},
		{
			name:       "explicit terragrunt.hcl.json drops terragrunt.hcl",
			configPath: filepath.Join("/repo", "unit", config.DefaultTerragruntJSONConfigPath),
			explicit:   true,
			expected:   []string{config.DefaultTerragruntJSONConfigPath, config.DefaultStackFile},
		},
		{
			name:       "explicit custom filename drops both defaults",
			configPath: filepath.Join("/repo", "unit", "main.hcl"),
			explicit:   true,
			expected:   []string{"main.hcl", config.DefaultStackFile},
		},
		{
			name:       "explicit stack filename keeps the defaults",
			configPath: config.DefaultStackFile,
			explicit:   true,
			expected:   discovery.DefaultConfigFilenames,
		},
	}

	for _, tt := range tc {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			opts := &options.TerragruntOptions{
				TerragruntConfigPath:              tt.configPath,
				TerragruntConfigPathExplicitlySet: tt.explicit,
			}

			assert.Equal(t, tt.expected, discovery.ConfigFilenames(opts))
		})
	}
}

// TestDiscovery_ExplicitConfigFilename pins which units are discovered when `--config` is set,
// in a tree that would otherwise be ambiguous.
func TestDiscovery_ExplicitConfigFilename(t *testing.T) {
	t.Parallel()

	root := venvtest.Root("/repo")
	files := map[string]string{
		"both/terragrunt.hcl":           "",
		"both/terragrunt.hcl.json":      "{}",
		"hcl-only/terragrunt.hcl":       "",
		"json-only/terragrunt.hcl.json": "{}",
		"custom/main.hcl":               "",
		"custom/terragrunt.hcl":         "",
	}

	tc := []struct {
		name       string
		configPath string
		expected   []string
	}{
		{
			name:       "terragrunt.hcl",
			configPath: config.DefaultTerragruntConfigPath,
			expected:   []string{"both", "hcl-only", "custom"},
		},
		{
			name:       "terragrunt.hcl.json",
			configPath: config.DefaultTerragruntJSONConfigPath,
			expected:   []string{"both", "json-only"},
		},
		{
			name:       "main.hcl",
			configPath: "main.hcl",
			expected:   []string{"custom"},
		},
	}

	for _, tt := range tc {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			opts := &options.TerragruntOptions{
				WorkingDir:                        root,
				TerragruntConfigPath:              filepath.Join(root, tt.configPath),
				TerragruntConfigPathExplicitlySet: true,
			}

			d := discovery.NewDiscovery(root).
				WithDiscoveryContext(&component.DiscoveryContext{WorkingDir: root}).
				WithConfigFilenames(discovery.ConfigFilenames(opts))

			components, err := d.Discover(
				config.WithCaches(t.Context()),
				logger.CreateLogger(),
				venv.MemVenv().WithFS(venvtest.NewFS(t, root, files)),
				new(config.Store),
				opts,
			)
			require.NoError(t, err)

			expected := make([]string, 0, len(tt.expected))
			for _, dir := range tt.expected {
				expected = append(expected, filepath.Join(root, dir))
			}

			assert.ElementsMatch(t, expected, components.Filter(component.UnitKind).Paths())

			for _, c := range components {
				assert.Equal(t, tt.configPath, c.ConfigFile())
			}
		})
	}
}
