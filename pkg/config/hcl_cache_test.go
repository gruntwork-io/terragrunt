package config_test

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
)

func TestParseConfigFileRereadsSameSizeRewriteOfRecentFile(t *testing.T) {
	t.Parallel()

	v, tmpDir := newMemTestDir(t)
	cfgPath := filepath.Join(tmpDir, config.DefaultTerragruntConfigPath)

	require.NoError(t, vfs.WriteFile(v.FS, cfgPath, []byte(`inputs = { value = "first" }`), 0o644))

	recent := time.Now().Add(time.Hour)
	require.NoError(t, v.FS.Chtimes(cfgPath, recent, recent))

	ctx := config.WithCaches(t.Context())

	before, _ := parseUnitInput(ctx, t, v, cfgPath)
	assert.Equal(t, "first", before)

	require.NoError(t, vfs.WriteFile(v.FS, cfgPath, []byte(`inputs = { value = "third" }`), 0o644))
	require.NoError(t, v.FS.Chtimes(cfgPath, recent, recent))

	after, _ := parseUnitInput(ctx, t, v, cfgPath)
	assert.Equal(t, "third", after)
}

func TestParseConfigFileReadsBackOnlyRecentFileOnCacheHit(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		modTime   time.Time
		name      string
		wantOpens int
	}{
		{name: "old file", modTime: time.Now().Add(-time.Hour), wantOpens: 1},
		{name: "recent file", modTime: time.Now().Add(time.Hour), wantOpens: 2},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			v, tmpDir := newMemTestDir(t)
			cfgPath := filepath.Join(tmpDir, config.DefaultTerragruntConfigPath)

			require.NoError(t, vfs.WriteFile(v.FS, cfgPath, []byte(`inputs = { value = "first" }`), 0o644))
			require.NoError(t, v.FS.Chtimes(cfgPath, tc.modTime, tc.modTime))

			fsys := &countingFS{FS: v.FS, opens: map[string]int{}}
			v = v.WithFS(fsys)
			ctx := config.WithCaches(t.Context())

			for range 2 {
				_, pctx := newTestParsingContext(t, cfgPath)

				cfg, err := config.ParseConfigFile(ctx, logger.CreateLogger(), v, pctx, cfgPath, nil)
				require.NoError(t, err)
				assert.Equal(t, "first", cfg.Inputs["value"])
			}

			assert.Equal(t, tc.wantOpens, fsys.opensOf(cfgPath))
		})
	}
}
