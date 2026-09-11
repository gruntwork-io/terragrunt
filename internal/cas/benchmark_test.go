package cas_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"golang.org/x/sync/errgroup"

	"github.com/gruntwork-io/terragrunt/internal/cas"
	"github.com/gruntwork-io/terragrunt/internal/git"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"
)

func BenchmarkClone(b *testing.B) {
	repoURL := startBenchServer(b)

	l := logger.CreateLogger()

	v := venvtest.NewOSWithEmptyEnv()

	b.Run("fresh clone", func(b *testing.B) {
		tempDir := b.TempDir()

		i := 0

		for b.Loop() {
			storePath := filepath.Join(tempDir, "store", strconv.Itoa(i))
			targetPath := filepath.Join(tempDir, "repo", strconv.Itoa(i))
			i++

			c, err := cas.New(venvtest.NewWithOSFS(), cas.WithStorePath(storePath))
			require.NoError(b, err)

			require.NoError(b, c.Clone(b.Context(), l, v, repoURL, cas.WithDir(targetPath),
				cas.WithDepth(-1)))
		}
	})

	b.Run("clone with existing store", func(b *testing.B) {
		tempDir := b.TempDir()
		storePath := filepath.Join(tempDir, "store")

		// First clone to populate store
		c, err := cas.New(venvtest.NewWithOSFS(), cas.WithStorePath(storePath))
		require.NoError(b, err)

		require.NoError(
			b,
			c.Clone(b.Context(), l, v, repoURL, cas.WithDir(filepath.Join(tempDir, "initial")),
				cas.WithDepth(-1)),
		)

		i := 0

		for b.Loop() {
			targetPath := filepath.Join(tempDir, "repo", strconv.Itoa(i))
			i++

			c, err := cas.New(venvtest.NewWithOSFS(), cas.WithStorePath(storePath))
			require.NoError(b, err)

			require.NoError(b, c.Clone(b.Context(), l, v, repoURL, cas.WithDir(targetPath),
				cas.WithDepth(-1)))
		}
	})
}

// BenchmarkConcurrentClone measures the `run --all` shape: many units,
// each with its own CAS over one shared store, cloning the same branch of
// one source at the same time. "warm tree" pre-populates the store so only
// the probe sits on the hot path; "cold store" starts each iteration from
// an empty store so the ingest does too.
func BenchmarkConcurrentClone(b *testing.B) {
	repoURL := startBenchServer(b)

	l := logger.CreateLogger()

	v := venvtest.NewOSWithEmptyEnv()

	callerCounts := []int{8, 32, 128}

	b.Run("warm tree", func(b *testing.B) {
		for _, callers := range callerCounts {
			b.Run(strconv.Itoa(callers)+" callers", func(b *testing.B) {
				tempDir := b.TempDir()
				storePath := filepath.Join(tempDir, "store")

				require.NoError(b, cloneConcurrently(b.Context(), l, v, storePath, repoURL,
					filepath.Join(tempDir, "warm"), 1))

				b.ReportAllocs()
				b.ResetTimer()

				for i := 0; b.Loop(); i++ {
					b.StopTimer()

					targetRoot := filepath.Join(tempDir, "repo", strconv.Itoa(i))

					b.StartTimer()

					require.NoError(b, cloneConcurrently(b.Context(), l, v, storePath, repoURL,
						targetRoot, callers))
				}
			})
		}
	})

	b.Run("cold store", func(b *testing.B) {
		for _, callers := range callerCounts {
			b.Run(strconv.Itoa(callers)+" callers", func(b *testing.B) {
				tempDir := b.TempDir()

				b.ReportAllocs()
				b.ResetTimer()

				for i := 0; b.Loop(); i++ {
					b.StopTimer()

					storePath := filepath.Join(tempDir, "store", strconv.Itoa(i))
					targetRoot := filepath.Join(tempDir, "repo", strconv.Itoa(i))

					b.StartTimer()

					require.NoError(b, cloneConcurrently(b.Context(), l, v, storePath, repoURL,
						targetRoot, callers))
				}
			})
		}
	})
}

// cloneConcurrently runs callers clones of main at once, each through a CAS
// of its own over storePath and into its own directory under targetRoot,
// and returns the first error any of them hit.
func cloneConcurrently(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	storePath, repoURL, targetRoot string,
	callers int,
) error {
	g, ctx := errgroup.WithContext(ctx)

	for i := range callers {
		g.Go(func() error {
			c, err := cas.New(venvtest.NewWithOSFS(), cas.WithStorePath(storePath))
			if err != nil {
				return err
			}

			return c.Clone(ctx, l, v, repoURL,
				cas.WithDir(filepath.Join(targetRoot, strconv.Itoa(i))),
				cas.WithBranch("main"),
				cas.WithDepth(-1))
		})
	}

	return g.Wait()
}

func BenchmarkContent(b *testing.B) {
	store := cas.NewStore(b.TempDir())

	content := cas.NewContent(store)

	// Prepare test data
	testData := []byte("test content for benchmarking")

	l := logger.CreateLogger()

	v := venvtest.NewOSWithEmptyEnv()

	b.Run("store", func(b *testing.B) {
		i := 0

		for b.Loop() {
			hash := "benchmark" + strconv.Itoa(i)
			i++

			require.NoError(b, content.Store(l, v, hash, testData, cas.StoredFilePerms))
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

				if err := content.Store(l, v, hash, testData, cas.StoredFilePerms); err != nil {
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

	g, err := git.NewGitRunner(venv.OSVenv())
	require.NoError(b, err)

	g = g.WithWorkDir(repoDir)

	ctx := b.Context()

	require.NoError(b, g.Clone(ctx, repoURL, false, 0, ""))

	b.Run("ls-remote", func(b *testing.B) {
		runner, err := git.NewGitRunner(venv.OSVenv())
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

	b.Run("cat-file --batch", func(b *testing.B) {
		tree, err := g.LsTreeRecursive(ctx, "HEAD")
		require.NoError(b, err)
		require.NotEmpty(b, tree.Entries(), "no entries in tree")

		hash := tree.Entries()[0].Hash

		tmpFile := b.TempDir() + "/cat-file"

		tmp, err := os.Create(tmpFile)
		require.NoError(b, err)

		defer os.Remove(tmpFile)
		defer tmp.Close()

		batch, err := g.StartCatFileBatch(ctx)
		require.NoError(b, err)

		// Not b.Cleanup. The benchmark's context is already canceled by the
		// time cleanups run, and Close reports that as a failure.
		defer func() { require.NoError(b, batch.Close()) }()

		b.ResetTimer()

		for b.Loop() {
			err := batch.ReadBlob(hash, tmp)
			require.NoError(b, err)
		}
	})
}

func startBenchServer(b *testing.B) string {
	b.Helper()

	srv, err := git.NewServer()
	require.NoError(b, err)

	b.Cleanup(func() { _ = srv.Close() })

	require.NoError(
		b,
		srv.CommitFile(b.Context(), "README.md", []byte("# test repo"), "add readme"),
	)
	require.NoError(
		b,
		srv.CommitFile(
			b.Context(),
			"main.tf",
			[]byte(`resource "null_resource" "test" {}`),
			"add main.tf",
		),
	)
	require.NoError(
		b,
		srv.CommitFile(
			b.Context(),
			"test/integration_test.go",
			[]byte("package test"),
			"add test file",
		),
	)

	url, err := srv.Start(b.Context())
	require.NoError(b, err)

	return url
}
