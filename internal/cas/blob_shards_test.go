package cas_test

import (
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/cas"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/stretchr/testify/assert"
)

func TestBlobShards(t *testing.T) {
	t.Parallel()

	t.Run("never outnumbers the blobs it serves", func(t *testing.T) {
		t.Parallel()

		store := t.TempDir()

		for _, pending := range []int{1, 2, 3} {
			assert.LessOrEqual(t, cas.BlobShards(vfs.NewOSFS(), store, pending), pending)
		}
	})

	t.Run("a large tree keeps the filesystem's ceiling", func(t *testing.T) {
		t.Parallel()

		store := t.TempDir()

		const plenty = 1000

		shards := cas.BlobShards(vfs.NewOSFS(), store, plenty)

		assert.GreaterOrEqual(t, shards, vfs.BTRFSWorkers)
		assert.LessOrEqual(t, shards, vfs.MaxFSWorkers,
			"a shard costs a git process, so the count stays bounded")
	})

	t.Run("an unprobeable store falls back to the default", func(t *testing.T) {
		t.Parallel()

		missing := filepath.Join(t.TempDir(), "no-such-store")

		assert.Equal(t, vfs.DefaultFSWorkers, cas.BlobShards(vfs.NewOSFS(), missing, 1000))
	})
}
