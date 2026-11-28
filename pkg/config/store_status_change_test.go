//go:build (linux && !mips && !mipsle && !mips64 && !mips64le) || darwin

package config_test

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/pkg/config/hclparse"
)

func TestStoreParseUnitASTSharesOneParseOfAnUnchangedFileOnDisk(t *testing.T) {
	t.Parallel()

	fsys := vfs.NewOSFS()
	path := filepath.Join(t.TempDir(), "root.hcl")
	require.NoError(t, vfs.WriteFile(fsys, path, []byte(`locals { a = 1 }`), 0o644))

	modTime := time.Now().Add(-time.Hour)
	require.NoError(t, fsys.Chtimes(path, modTime, modTime))

	s := &config.Store{}

	first, err := s.ParseUnitAST(t.Context(), fsys, path, stat(t, fsys, path), hclparse.NewParser())
	require.NoError(t, err)

	second, err := s.ParseUnitAST(t.Context(), fsys, path, stat(t, fsys, path), hclparse.NewParser())
	require.NoError(t, err)

	assert.Same(t, first.File, second.File)
	assert.Equal(t, config.Stats{Hits: 1, Misses: 1}, s.Stats())
}

func TestStoreParseUnitASTReadsBackAFileWithARecentStatusChangeOnAHit(t *testing.T) {
	t.Parallel()

	osFS := vfs.NewOSFS()
	path := filepath.Join(t.TempDir(), "root.hcl")
	require.NoError(t, vfs.WriteFile(osFS, path, []byte(`locals { a = 1 }`), 0o644))

	modTime := time.Now().Add(-time.Hour)
	require.NoError(t, osFS.Chtimes(path, modTime, modTime))

	fsys := &countingFS{FS: osFS, opens: map[string]int{}}
	s := &config.Store{}

	for range 2 {
		_, err := s.ParseUnitAST(t.Context(), fsys, path, stat(t, fsys, path), hclparse.NewParser())
		require.NoError(t, err)
	}

	assert.Equal(t, config.Stats{Hits: 1, Misses: 1}, s.Stats())
	assert.Equal(t, 2, fsys.opensOf(path), "the hit compares the file with the parsed bytes")
}

func TestStoreParseUnitASTParsesAgainWhenASameSizeRewriteRestoresAnOldModTime(t *testing.T) {
	t.Parallel()

	fsys := vfs.NewOSFS()
	dir := t.TempDir()
	path := filepath.Join(dir, "root.hcl")
	require.NoError(t, vfs.WriteFile(fsys, path, []byte(`locals { a = 1 }`), 0o644))

	modTime := time.Now().Add(-time.Hour)
	require.NoError(t, fsys.Chtimes(path, modTime, modTime))

	s := &config.Store{}

	_, err := s.ParseUnitAST(t.Context(), fsys, path, stat(t, fsys, path), hclparse.NewParser())
	require.NoError(t, err)

	rewrite := filepath.Join(dir, "root.hcl.new")
	require.NoError(t, vfs.WriteFile(fsys, rewrite, []byte(`locals { a = 2 }`), 0o644))
	require.NoError(t, fsys.Chtimes(rewrite, modTime, modTime))
	require.NoError(t, vfs.RenameOver(fsys, rewrite, path))

	second, err := s.ParseUnitAST(t.Context(), fsys, path, stat(t, fsys, path), hclparse.NewParser())
	require.NoError(t, err)

	assert.Equal(t, `locals { a = 2 }`, second.Content())
	assert.Equal(t, config.Stats{Hits: 0, Misses: 2}, s.Stats(), "the stat alone shows the rewrite")
}

func TestResolveRunConfigReadsASameSizeRewriteThatRestoresAnOldModTime(t *testing.T) {
	t.Parallel()

	v := venv.MemVenv().WithFS(vfs.NewOSFS())
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, config.DefaultTerragruntConfigPath)
	require.NoError(t, vfs.WriteFile(v.FS, cfgPath, []byte(`inputs = { value = "AAAA" }`), 0o644))

	modTime := time.Now().Add(-time.Hour)
	require.NoError(t, v.FS.Chtimes(cfgPath, modTime, modTime))

	ctx := config.WithCaches(t.Context())
	s := new(config.Store)

	before, _ := parseUnitInput(ctx, t, v, s, cfgPath)
	assert.Equal(t, "AAAA", before)

	rewrite := filepath.Join(dir, "terragrunt.hcl.new")
	require.NoError(t, vfs.WriteFile(v.FS, rewrite, []byte(`inputs = { value = "BBBB" }`), 0o644))
	require.NoError(t, v.FS.Chtimes(rewrite, modTime, modTime))
	require.NoError(t, vfs.RenameOver(v.FS, rewrite, cfgPath))

	after, _ := parseUnitInput(ctx, t, v, s, cfgPath)
	assert.Equal(t, "BBBB", after)
}

func TestReadTerragruntConfigRereadsASameSizeRewriteThatRestoresAnOldModTime(t *testing.T) {
	t.Parallel()

	v := venv.MemVenv().WithFS(vfs.NewOSFS())
	rootDir := t.TempDir()
	units := writeReadConfigFixture(t, v, rootDir, `locals { value = "first" }`, nil)

	commonPath := filepath.Join(rootDir, "common.hcl")
	modTime := stat(t, v.FS, commonPath).ModTime()

	ctx := config.WithCaches(t.Context())
	s := new(config.Store)

	before, _ := parseUnitInput(ctx, t, v, s, units[0])
	assert.Equal(t, "first", before)

	rewrite := filepath.Join(rootDir, "common.hcl.new")
	require.NoError(t, vfs.WriteFile(v.FS, rewrite, []byte(`locals { value = "third" }`), 0o644))
	require.NoError(t, v.FS.Chtimes(rewrite, modTime, modTime))
	require.NoError(t, vfs.RenameOver(v.FS, rewrite, commonPath))

	after, _ := parseUnitInput(ctx, t, v, s, units[1])
	assert.Equal(t, "third", after)
}
