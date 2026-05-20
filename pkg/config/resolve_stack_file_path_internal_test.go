package config

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestResolveStackFilePath pins resolveStackFilePath across dependency-target shapes (direct stack file, explicit terragrunt config, bare directory).
func TestResolveStackFilePath(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	wantStack := filepath.Join(tmpDir, DefaultStackFile)

	cases := []struct {
		name   string
		raw    string
		target string
		want   string
		wantOK bool
	}{
		{"stackFileDirectly", filepath.Join(tmpDir, DefaultStackFile), wantStack, wantStack, true},
		{"explicitTerragruntHCL", filepath.Join(tmpDir, DefaultTerragruntConfigPath), filepath.Join(tmpDir, DefaultTerragruntConfigPath), "", false},
		{"explicitTerragruntJSON", filepath.Join(tmpDir, DefaultTerragruntJSONConfigPath), filepath.Join(tmpDir, DefaultTerragruntJSONConfigPath), "", false},
		{"bareDirectory", tmpDir, filepath.Join(tmpDir, DefaultTerragruntConfigPath), wantStack, true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, ok := resolveStackFilePath(tc.raw, tc.target)
			assert.Equal(t, tc.wantOK, ok)
			assert.Equal(t, tc.want, got)
		})
	}
}

// FuzzResolveStackFilePath verifies the path-rewrite helper never panics and every handled candidate ends in DefaultStackFile. raw is the user-authored dependency.config_path; target is the post-cleanup absolute path the resolver actually inspects - the two diverge in production (bare-directory raw + cleaned target with appended DefaultTerragruntConfigPath), so fuzz them independently.
func FuzzResolveStackFilePath(f *testing.F) {
	seeds := [][2]string{
		{"/abs/dir/" + DefaultStackFile, "/abs/dir/" + DefaultStackFile},
		{"/abs/dir", "/abs/dir/" + DefaultTerragruntConfigPath},
		{"/abs/dir", "/abs/dir/" + DefaultTerragruntJSONConfigPath},
		{"/abs/dir/" + DefaultTerragruntConfigPath, "/abs/dir/" + DefaultTerragruntConfigPath},
		{"relative/dir", "relative/dir/" + DefaultTerragruntConfigPath},
		{"", ""},
		{".", "./" + DefaultTerragruntConfigPath},
		{"/", "/" + DefaultTerragruntConfigPath},
		{"\x00", "\x00"},
		{"unicode/café", "unicode/café/" + DefaultTerragruntConfigPath},
	}

	for _, seed := range seeds {
		f.Add(seed[0], seed[1])
	}

	f.Fuzz(func(t *testing.T, raw, target string) {
		got, ok := resolveStackFilePath(raw, target)
		if !ok {
			require.Empty(t, got, "resolveStackFilePath must return empty string when ok=false (raw=%q target=%q got=%q)", raw, target, got)
			return
		}

		require.Equal(t, DefaultStackFile, filepath.Base(got), "resolveStackFilePath must return a path whose base is %s when ok=true (raw=%q target=%q got=%q)", DefaultStackFile, raw, target, got)
	})
}
