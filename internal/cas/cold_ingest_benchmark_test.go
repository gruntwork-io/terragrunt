package cas_test

import (
	"fmt"
	"math/rand/v2"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/gruntwork-io/terragrunt/internal/cas"
	"github.com/gruntwork-io/terragrunt/internal/git"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"
)

const (
	// coldIngestMinSize is the smallest a fixture file gets. With
	// [coldIngestSizeSpread] it puts every file in the 64..512 byte band,
	// where the per-blob overhead outweighs the copy itself.
	coldIngestMinSize = 64

	// coldIngestSizeSpread is how far above [coldIngestMinSize] a fixture
	// file's size may reach.
	coldIngestSizeSpread = 449

	// coldIngestSeed keeps the fixture identical across runs and trees so
	// benchstat compares the same repository.
	coldIngestSeed = 0x7a11

	coldIngestTopDirs = 17
	coldIngestSubDirs = 13

	// coldIngestAlphabet is the printable content of every fixture file.
	coldIngestAlphabet = "abcdefghijklmnopqrstuvwxyz0123456789 \n"
)

// BenchmarkColdIngest clones repositories of increasing size into a fresh
// store on every iteration, so each blob takes the cold path from git into
// the store. The timed region includes the fetch and the tree listing.
// Only the store and target directories are created outside it.
func BenchmarkColdIngest(b *testing.B) {
	l := logger.CreateLogger()
	v := venvtest.NewOSWithEmptyEnv()

	for _, fileCount := range []int{200, 1000, 3000} {
		b.Run("files="+strconv.Itoa(fileCount), func(b *testing.B) {
			repoURL := startColdIngestServer(b, fileCount)
			tempDir := b.TempDir()

			b.ReportAllocs()
			b.ResetTimer()

			for i := 0; b.Loop(); i++ {
				b.StopTimer()

				storePath := filepath.Join(tempDir, "store", strconv.Itoa(i))
				targetPath := filepath.Join(tempDir, "repo", strconv.Itoa(i))

				c, err := cas.New(venvtest.NewWithOSFS(), cas.WithStorePath(storePath))
				require.NoError(b, err)

				b.StartTimer()

				require.NoError(b, c.Clone(b.Context(), l, v, repoURL, cas.WithDir(targetPath),
					cas.WithDepth(-1)))
			}
		})
	}
}

// startColdIngestServer serves a repository whose single commit has
// fileCount distinct small files spread across nested directories.
func startColdIngestServer(b *testing.B, fileCount int) string {
	b.Helper()

	srv, err := git.NewServer()
	require.NoError(b, err)

	b.Cleanup(func() { require.NoError(b, srv.Close()) })

	require.NoError(b, srv.CommitFiles(b.Context(), coldIngestBenchFixture(fileCount), "cold ingest bench fixture"))

	url, err := srv.Start(b.Context())
	require.NoError(b, err)

	return url
}

// coldIngestBenchFixture builds fileCount files of deterministic content.
// Every file embeds its own index so no two dedupe to the same blob.
func coldIngestBenchFixture(fileCount int) map[string][]byte {
	rng := rand.New(rand.NewPCG(coldIngestSeed, coldIngestSeed))
	files := make(map[string][]byte, fileCount)

	for i := range fileCount {
		path := fmt.Sprintf("d%02d/s%02d/file%05d.txt", i%coldIngestTopDirs, (i/coldIngestTopDirs)%coldIngestSubDirs, i)

		size := coldIngestMinSize + rng.IntN(coldIngestSizeSpread)
		content := make([]byte, 0, size)
		content = append(content, []byte("file "+strconv.Itoa(i)+"\n")...)

		for len(content) < size {
			content = append(content, coldIngestAlphabet[rng.IntN(len(coldIngestAlphabet))])
		}

		files[path] = content
	}

	return files
}
