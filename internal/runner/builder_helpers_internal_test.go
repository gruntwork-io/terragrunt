package runner

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/gruntwork-io/terragrunt/internal/discovery"
	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/pkg/options"
)

// TestBuildConfigFilenames covers how `--config` interacts with the default config filenames.
// A custom filename has to replace the default unit filenames rather than join them, otherwise a
// directory holding both the custom file and a default one yields two unit components and
// discovery rejects it as ambiguous.
func TestBuildConfigFilenames(t *testing.T) {
	t.Parallel()

	tc := []struct {
		name       string
		configPath string
		expected   []string
	}{
		{
			name:       "unset config path keeps the defaults",
			configPath: "",
			expected:   discovery.DefaultConfigFilenames,
		},
		{
			name:       "default hcl config path keeps the defaults",
			configPath: filepath.Join("/repo", "unit", config.DefaultTerragruntConfigPath),
			expected:   discovery.DefaultConfigFilenames,
		},
		{
			name:       "default json config path keeps the defaults",
			configPath: filepath.Join("/repo", "unit", config.DefaultTerragruntJSONConfigPath),
			expected:   discovery.DefaultConfigFilenames,
		},
		{
			name:       "custom config filename takes precedence over the default unit filenames",
			configPath: filepath.Join("/repo", "unit", "not-terragrunt.hcl"),
			expected:   []string{"not-terragrunt.hcl", config.DefaultStackFile},
		},
		{
			name:       "custom json config filename takes precedence over the default unit filenames",
			configPath: filepath.Join("/repo", "unit", "not-terragrunt.hcl.json"),
			expected:   []string{"not-terragrunt.hcl.json", config.DefaultStackFile},
		},
	}

	for _, tt := range tc {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			opts := &options.TerragruntOptions{TerragruntConfigPath: tt.configPath}

			assert.Equal(t, tt.expected, buildConfigFilenames(opts))
		})
	}
}
