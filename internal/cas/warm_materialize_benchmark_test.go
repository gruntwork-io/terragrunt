package cas_test

import (
	"path/filepath"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/gruntwork-io/terragrunt/internal/cas"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"
)

// BenchmarkWarmMaterialize clones into a store that already holds every
// blob, so the timed region is dominated by materializing the tree into
// a fresh target rather than by ingest.
func BenchmarkWarmMaterialize(b *testing.B) {
	l := logger.CreateLogger()
	v := venvtest.NewOSWithEmptyEnv()

	repoURL := startColdIngestServer(b, 3000)
	tempDir := b.TempDir()
	storePath := filepath.Join(tempDir, "store")

	warm, err := cas.New(venvtest.NewWithOSFS(), cas.WithStorePath(storePath))
	require.NoError(b, err)

	require.NoError(b, warm.Clone(b.Context(), l, v, repoURL,
		cas.WithDir(filepath.Join(tempDir, "initial")), cas.WithDepth(-1)))

	target := filepath.Join(tempDir, "target")

	i := 0

	for b.Loop() {
		c, err := cas.New(venvtest.NewWithOSFS(), cas.WithStorePath(storePath))
		require.NoError(b, err)

		require.NoError(b, c.Clone(b.Context(), l, v, repoURL,
			cas.WithDir(filepath.Join(target, strconv.Itoa(i))), cas.WithDepth(-1)))

		i++
	}
}
