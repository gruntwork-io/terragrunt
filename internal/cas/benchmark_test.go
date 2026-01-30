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
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
)

func BenchmarkClone(b *testing.B) {
	// Use a small, public repository for consistent results
	repo := "https://github.com/gruntwork-io/terragrunt.git"

	l := logger.CreateLogger()

	b.Run("fresh clone", func(b *testing.B) {
		tempDir := b.TempDir()

		b.ResetTimer()

		for i := 0; b.Loop(); i++ {
			b.StopTimer()

			storePath := filepath.Join(tempDir, "store", strconv.Itoa(i))
			targetPath := filepath.Join(tempDir, "repo", strconv.Itoa(i))

			c, err := cas.New(cas.Options{
				StorePath: storePath,
			})
			if err != nil {
				b.Fatal(err)
			}

			b.StartTimer()

			if err := c.Clone(b.Context(), l, &cas.CloneOptions{
				Dir: targetPath,
			}, repo); err != nil {
				b.Fatal(err)
			}
		}
	})

	b.Run("clone with existing store", func(b *testing.B) {
		tempDir := b.TempDir()
		storePath := filepath.Join(tempDir, "store")

		// First clone to populate store
		c, err := cas.New(cas.Options{
			StorePath: storePath,
		})
		if err != nil {
			b.Fatal(err)
		}

		if err := c.Clone(b.Context(), l, &cas.CloneOptions{
			Dir: filepath.Join(tempDir, "initial"),
		}, repo); err != nil {
			b.Fatal(err)
		}

		b.ResetTimer()

		for i := 0; b.Loop(); i++ {
			b.StopTimer()

			targetPath := filepath.Join(tempDir, "repo", strconv.Itoa(i))

			c, err := cas.New(cas.Options{
				StorePath: storePath,
			})
			if err != nil {
				b.Fatal(err)
			}

			b.StartTimer()

			if err := c.Clone(b.Context(), l, &cas.CloneOptions{
				Dir: targetPath,
			}, repo); err != nil {
				b.Fatal(err)
			}
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

			if err := content.Store(l, hash, testData); err != nil {
				b.Fatal(err)
			}
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
	// Setup a git repository for testing
	repoDir := b.TempDir()

	g, err := git.NewGitRunner()
	if err != nil {
		b.Fatal(err)
	}

	g = g.WithWorkDir(repoDir)

	ctx := b.Context()

	if err = g.Clone(ctx, "https://github.com/gruntwork-io/terragrunt.git", false, 1, "main"); err != nil {
		b.Fatal(err)
	}

	b.Run("ls-remote", func(b *testing.B) {
		g, err = git.NewGitRunner()
		if err != nil {
			b.Fatal(err)
		}

		g = g.WithWorkDir(repoDir)

		b.ResetTimer()

		for b.Loop() {
			_, err := g.LsRemote(ctx, "https://github.com/gruntwork-io/terragrunt.git", "HEAD")
			if err != nil {
				b.Fatal(err)
			}
		}
	})

	b.Run("ls-tree -r", func(b *testing.B) {
		b.ResetTimer()

		for b.Loop() {
			_, err := g.LsTreeRecursive(ctx, "HEAD")
			if err != nil {
				b.Fatal(err)
			}
		}
	})

	b.Run("cat-file", func(b *testing.B) {
		// First get a valid hash
		tree, err := g.LsTreeRecursive(ctx, "HEAD")
		if err != nil {
			b.Fatal(err)
		}

		if len(tree.Entries()) == 0 {
			b.Fatal("no entries in tree")
		}

		hash := tree.Entries()[0].Hash

		tmpFile := b.TempDir() + "/cat-file"

		tmp, err := os.Create(tmpFile)
		if err != nil {
			b.Fatal(err)
		}

		defer os.Remove(tmpFile)
		defer tmp.Close()

		b.ResetTimer()

		for b.Loop() {
			err := g.CatFile(ctx, hash, tmp)
			if err != nil {
				b.Fatal(err)
			}
		}
	})
}
