package config_test

import (
	"path/filepath"
	"strconv"
	"testing"

	"github.com/gruntwork-io/terragrunt/pkg/config"
)

// BenchmarkFilesReadAdd measures accumulation cost at sizes representative of
// walking local module directories in a monorepo. "unique" adds n distinct
// paths into a fresh set; "duplicate" re-adds n already-recorded paths, the
// all-dedup-hit pattern seen when the same module dir is marked repeatedly.
func BenchmarkFilesReadAdd(b *testing.B) {
	for _, size := range []int{1000, 5000, 20000} {
		paths := benchPaths(size)

		b.Run("unique/"+strconv.Itoa(size), func(b *testing.B) {
			for b.Loop() {
				f := config.NewFilesRead()
				for _, path := range paths {
					f.Add(path)
				}
			}
		})

		b.Run("duplicate/"+strconv.Itoa(size), func(b *testing.B) {
			f := config.NewFilesRead()
			for _, path := range paths {
				f.Add(path)
			}

			for b.Loop() {
				for _, path := range paths {
					f.Add(path)
				}
			}
		})
	}
}

// benchPaths fabricates n distinct module-file paths.
func benchPaths(n int) []string {
	paths := make([]string, n)
	for i := range n {
		paths[i] = filepath.Join("modules", strconv.Itoa(i%100), "file-"+strconv.Itoa(i)+".tf")
	}

	return paths
}
