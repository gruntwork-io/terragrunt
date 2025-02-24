package clngo_test

import (
	"fmt"
	"path/filepath"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/gruntwork-io/terragrunt/internal/clngo"
)

func BenchmarkClone(b *testing.B) {
	// Use a small, public repository for consistent results
	repo := "https://github.com/yhakbar/cln.git"

	b.Run("fresh clone", func(b *testing.B) {
		tempDir := b.TempDir()

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			storePath := filepath.Join(tempDir, "store", strconv.Itoa(i))
			targetPath := filepath.Join(tempDir, "repo", strconv.Itoa(i))

			cln, err := clngo.New(repo, clngo.Options{
				Dir:       targetPath,
				StorePath: storePath,
			})
			if err != nil {
				b.Fatal(err)
			}

			if err := cln.Clone(); err != nil {
				b.Fatal(err)
			}
		}
	})

	b.Run("clone with existing store", func(b *testing.B) {
		tempDir := b.TempDir()
		storePath := filepath.Join(tempDir, "store")

		// First clone to populate store
		cln, err := clngo.New(repo, clngo.Options{
			Dir:       filepath.Join(tempDir, "initial"),
			StorePath: storePath,
		})
		if err != nil {
			b.Fatal(err)
		}
		if err := cln.Clone(); err != nil {
			b.Fatal(err)
		}

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			targetPath := filepath.Join(tempDir, "repo", strconv.Itoa(i))

			cln, err := clngo.New(repo, clngo.Options{
				Dir:       targetPath,
				StorePath: storePath,
			})
			if err != nil {
				b.Fatal(err)
			}

			if err := cln.Clone(); err != nil {
				b.Fatal(err)
			}
		}
	})
}

func BenchmarkContent(b *testing.B) {
	store, err := clngo.NewStore(b.TempDir())
	if err != nil {
		b.Fatal(err)
	}
	content := clngo.NewContent(store)

	// Prepare test data
	testData := []byte("test content for benchmarking")

	b.Run("store", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			hash := fmt.Sprintf("benchmark%d", i)
			if err := content.Store(hash, testData); err != nil {
				b.Fatal(err)
			}
		}
	})

	b.Run("batch_store", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			items := make(map[string][]byte, 100)
			for j := 0; j < 100; j++ {
				hash := fmt.Sprintf("benchmark%d_%d", i, j)
				items[hash] = testData
			}
			if err := content.StoreBatch(items); err != nil {
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

				if err := content.Store(hash, testData); err != nil {
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
	git := clngo.NewGitRunner().WithWorkDir(repoDir)
	if err := git.Clone("https://github.com/yhakbar/cln.git", false, 1, "main"); err != nil {
		b.Fatal(err)
	}

	b.Run("ls-remote", func(b *testing.B) {
		git := clngo.NewGitRunner() // No workDir needed for ls-remote
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, err := git.LsRemote("https://github.com/yhakbar/cln.git", "HEAD")
			if err != nil {
				b.Fatal(err)
			}
		}
	})

	b.Run("ls-tree", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, err := git.LsTree("HEAD", ".")
			if err != nil {
				b.Fatal(err)
			}
		}
	})

	b.Run("cat-file", func(b *testing.B) {
		// First get a valid hash
		tree, err := git.LsTree("HEAD", ".")
		if err != nil {
			b.Fatal(err)
		}
		if len(tree.Entries()) == 0 {
			b.Fatal("no entries in tree")
		}
		hash := tree.Entries()[0].Hash

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, err := git.CatFile(hash)
			if err != nil {
				b.Fatal(err)
			}
		}
	})
}
