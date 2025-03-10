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

	"github.com/gruntwork-io/terragrunt/internal/cas"
	"github.com/gruntwork-io/terragrunt/pkg/log"
)

func BenchmarkClone(b *testing.B) {
	// Use a small, public repository for consistent results
	repo := "https://github.com/gruntwork-io/terragrunt.git"

	l := log.New()

	b.Run("fresh clone", func(b *testing.B) {
		tempDir := b.TempDir()

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			storePath := filepath.Join(tempDir, "store", strconv.Itoa(i))
			targetPath := filepath.Join(tempDir, "repo", strconv.Itoa(i))

			c, err := cas.New(cas.Options{
				StorePath: storePath,
			})
			if err != nil {
				b.Fatal(err)
			}

			if err := c.Clone(context.TODO(), &l, cas.CloneOptions{
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
		if err := c.Clone(context.TODO(), &l, cas.CloneOptions{
			Dir: filepath.Join(tempDir, "initial"),
		}, repo); err != nil {
			b.Fatal(err)
		}

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			targetPath := filepath.Join(tempDir, "repo", strconv.Itoa(i))

			c, err := cas.New(cas.Options{
				StorePath: storePath,
			})
			if err != nil {
				b.Fatal(err)
			}

			if err := c.Clone(context.TODO(), &l, cas.CloneOptions{
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

	l := log.New()

	b.Run("store", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			hash := fmt.Sprintf("benchmark%d", i)
			if err := content.Store(&l, hash, testData); err != nil {
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

				if err := content.Store(&l, hash, testData); err != nil {
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
	git := cas.NewGitRunner().WithWorkDir(repoDir)

	ctx := context.Background()

	if err := git.Clone(ctx, "https://github.com/gruntwork-io/terragrunt.git", false, 1, "main"); err != nil {
		b.Fatal(err)
	}

	b.Run("ls-remote", func(b *testing.B) {
		git := cas.NewGitRunner() // No workDir needed for ls-remote
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, err := git.LsRemote(ctx, "https://github.com/gruntwork-io/terragrunt.git", "HEAD")
			if err != nil {
				b.Fatal(err)
			}
		}
	})

	b.Run("ls-tree", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, err := git.LsTree(ctx, "HEAD", ".")
			if err != nil {
				b.Fatal(err)
			}
		}
	})

	b.Run("cat-file", func(b *testing.B) {
		// First get a valid hash
		tree, err := git.LsTree(ctx, "HEAD", ".")
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
		for i := 0; i < b.N; i++ {
			err := git.CatFile(ctx, hash, tmp)
			if err != nil {
				b.Fatal(err)
			}
		}
	})
}
