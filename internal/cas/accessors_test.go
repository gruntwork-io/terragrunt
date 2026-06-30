package cas_test

import (
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/cas"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCASStoreAccessors(t *testing.T) {
	t.Parallel()

	storePath := t.TempDir()

	c, err := cas.New(cas.WithStorePath(storePath))
	require.NoError(t, err)

	assert.Equal(t, storePath, c.StorePath())
	assert.Equal(t, filepath.Join(storePath, "blobs"), c.BlobStore().Path())
	assert.Equal(t, filepath.Join(storePath, "synth", "trees"), c.SynthStore().Path())
}

func TestGitStoreRootPath(t *testing.T) {
	t.Parallel()

	rootPath := t.TempDir()
	assert.Equal(t, rootPath, cas.NewGitStore(rootPath).RootPath())
}
