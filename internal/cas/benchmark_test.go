package cas_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/gruntwork-io/terragrunt/internal/cas"
	"github.com/gruntwork-io/terragrunt/internal/git"
	"github.com/gruntwork-io/terragrunt/internal/vexec"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/stretchr/testify/require"
)

func BenchmarkClone(b *testing.B) {
	repoURL := startBenchServer(b)

	l := logger.CreateLogger()

	b.Run("fresh clone", func(b *testing.B) {
		tempDir := b.TempDir()

		b.ResetTimer()

		for i := 0; b.Loop(); i++ {
			b.StopTimer()

			storePath := filepath.Join(tempDir, "store", strconv.Itoa(i))
			targetPath := filepath.Join(tempDir, "repo", strconv.Itoa(i))

			c, err := cas.New(cas.WithStorePath(storePath))
			require.NoError(b, err)

			b.StartTimer()

			require.NoError(b, c.Clone(b.Context(), l, &cas.CloneOptions{
				Dir:   targetPath,
				Depth: -1,
			}, repoURL))
		}
	})

	b.Run("clone with existing store", func(b *testing.B) {
		tempDir := b.TempDir()
		storePath := filepath.Join(tempDir, "store")

		// First clone to populate store
		c, err := cas.New(cas.WithStorePath(storePath))
		require.NoError(b, err)

		require.NoError(b, c.Clone(b.Context(), l, &cas.CloneOptions{
			Dir:   filepath.Join(tempDir, "initial"),
			Depth: -1,
		}, repoURL))

		b.ResetTimer()

		for i := 0; b.Loop(); i++ {
			b.StopTimer()

			targetPath := filepath.Join(tempDir, "repo", strconv.Itoa(i))

			c, err := cas.New(cas.WithStorePath(storePath))
			require.NoError(b, err)

			b.StartTimer()

			require.NoError(b, c.Clone(b.Context(), l, &cas.CloneOptions{
				Dir:   targetPath,
				Depth: -1,
			}, repoURL))
		}
	})
}

func BenchmarkContent(b *testing.B) {
	store := cas.NewStore(b.TempDir())

	content := cas.NewContent(store)

	// Prepare test data
	testData := []byte("test content for benchmarking")

	l := logger.CreateLogger()

	b.Run("store", func(b *testing.B) {
		for i := 0; b.Loop(); i++ {
			b.StopTimer()

			hash := "benchmark" + strconv.Itoa(i)

			b.StartTimer()

			require.NoError(b, content.Store(l, hash, testData))
		}
	})

	b.Run("parallel_store", func(b *testing.B) {
		var mu sync.Mutex

		seen := make(map[string]bool)

		b.RunParallel(func(pb *testing.PB) {
			i := 0
			for pb.Next() {
				// Generate unique hash for each goroutine iteration
				hash := fmt.Sprintf("benchmark%d_%d_%d", b.N, i, time.Now().UnixNano())

				mu.Lock()

				if seen[hash] {
					mu.Unlock()
					continue
				}

				seen[hash] = true

				mu.Unlock()

				if err := content.Store(l, hash, testData); err != nil {
					b.Fatal(err)
				}

				i++
			}
		})
	})
}

func BenchmarkGitOperations(b *testing.B) {
	repoURL := startBenchServer(b)

	// Clone the repo locally for tree operations
	repoDir := b.TempDir()

	g, err := git.NewGitRunner(vexec.NewOSExec())
	require.NoError(b, err)

	g = g.WithWorkDir(repoDir)

	ctx := b.Context()

	require.NoError(b, g.Clone(ctx, repoURL, false, 0, ""))

	b.Run("ls-remote", func(b *testing.B) {
		runner, err := git.NewGitRunner(vexec.NewOSExec())
		require.NoError(b, err)

		b.ResetTimer()

		for b.Loop() {
			_, err := runner.LsRemote(ctx, repoURL, "HEAD")
			require.NoError(b, err)
		}
	})

	b.Run("ls-tree -r", func(b *testing.B) {
		b.ResetTimer()

		for b.Loop() {
			_, err := g.LsTreeRecursive(ctx, "HEAD")
			require.NoError(b, err)
		}
	})

	b.Run("cat-file", func(b *testing.B) {
		tree, err := g.LsTreeRecursive(ctx, "HEAD")
		require.NoError(b, err)
		require.NotEmpty(b, tree.Entries(), "no entries in tree")

		hash := tree.Entries()[0].Hash

		tmpFile := b.TempDir() + "/cat-file"

		tmp, err := os.Create(tmpFile)
		require.NoError(b, err)

		defer os.Remove(tmpFile)
		defer tmp.Close()

		b.ResetTimer()

		for b.Loop() {
			err := g.CatFile(ctx, hash, tmp)
			require.NoError(b, err)
		}
	})
}

func startBenchServer(b *testing.B) string {
	b.Helper()

	srv, err := git.NewServer()
	require.NoError(b, err)

	b.Cleanup(func() { _ = srv.Close() })

	require.NoError(b, srv.CommitFile("README.md", []byte("# test repo"), "add readme"))
	require.NoError(b, srv.CommitFile("main.tf", []byte(`resource "null_resource" "test" {}`), "add main.tf"))
	require.NoError(b, srv.CommitFile("test/integration_test.go", []byte("package test"), "add test file"))

	url, err := srv.Start(b.Context())
	require.NoError(b, err)

	return url
}
