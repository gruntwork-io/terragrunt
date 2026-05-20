package config_test

import (
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestResolveStackFilePath pins ResolveStackFilePath across dependency-target shapes (direct stack file, explicit terragrunt config, bare directory).
func TestResolveStackFilePath(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	wantStack := filepath.Join(tmpDir, config.DefaultStackFile)

	cases := []struct {
		name   string
		raw    string
		target string
		want   string
		wantOK bool
	}{
		{"stackFileDirectly", filepath.Join(tmpDir, config.DefaultStackFile), wantStack, wantStack, true},
		{"explicitTerragruntHCL", filepath.Join(tmpDir, config.DefaultTerragruntConfigPath), filepath.Join(tmpDir, config.DefaultTerragruntConfigPath), "", false},
		{"explicitTerragruntJSON", filepath.Join(tmpDir, config.DefaultTerragruntJSONConfigPath), filepath.Join(tmpDir, config.DefaultTerragruntJSONConfigPath), "", false},
		{"bareDirectory", tmpDir, filepath.Join(tmpDir, config.DefaultTerragruntConfigPath), wantStack, true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, ok := config.ResolveStackFilePath(tc.raw, tc.target)
			assert.Equal(t, tc.wantOK, ok)
			assert.Equal(t, tc.want, got)
		})
	}
}

// FuzzResolveStackFilePath verifies the path-rewrite helper never panics and every handled candidate ends in DefaultStackFile. raw is the user-authored dependency.config_path; target is the post-cleanup absolute path the resolver actually inspects - the two diverge in production (bare-directory raw + cleaned target with appended DefaultTerragruntConfigPath), so fuzz them independently.
func FuzzResolveStackFilePath(f *testing.F) {
	seeds := [][2]string{
		{"/abs/dir/" + config.DefaultStackFile, "/abs/dir/" + config.DefaultStackFile},
		{"/abs/dir", "/abs/dir/" + config.DefaultTerragruntConfigPath},
		{"/abs/dir", "/abs/dir/" + config.DefaultTerragruntJSONConfigPath},
		{"/abs/dir/" + config.DefaultTerragruntConfigPath, "/abs/dir/" + config.DefaultTerragruntConfigPath},
		{"relative/dir", "relative/dir/" + config.DefaultTerragruntConfigPath},
		{"", ""},
		{".", "./" + config.DefaultTerragruntConfigPath},
		{"/", "/" + config.DefaultTerragruntConfigPath},
		{"\x00", "\x00"},
		{"unicode/café", "unicode/café/" + config.DefaultTerragruntConfigPath},
	}

	for _, seed := range seeds {
		f.Add(seed[0], seed[1])
	}

	f.Fuzz(func(t *testing.T, raw, target string) {
		got, ok := config.ResolveStackFilePath(raw, target)
		if !ok {
			require.Empty(t, got, "ResolveStackFilePath must return empty string when ok=false (raw=%q target=%q got=%q)", raw, target, got)
			return
		}

		require.Equal(t, config.DefaultStackFile, filepath.Base(got), "ResolveStackFilePath must return a path whose base is %s when ok=true (raw=%q target=%q got=%q)", config.DefaultStackFile, raw, target, got)
	})
}
