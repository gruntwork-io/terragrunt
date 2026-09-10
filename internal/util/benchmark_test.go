package util_test

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/util"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
)

// buildCopyTree writes a tree shaped like a module cache: nested
// directories of small config files plus a few provider-sized blobs.
func buildCopyTree(tb testing.TB, root string, dirs, filesPerDir, fileSize, blobs, blobSize int) {
	tb.Helper()

	const (
		fanout   = 8
		dirPerms = 0o755
		perms    = 0o644
	)

	small := make([]byte, fileSize)

	for i := range dirs {
		dir := filepath.Join(root, strconv.Itoa(i/(fanout*fanout)), strconv.Itoa(i/fanout), strconv.Itoa(i))
		if err := os.MkdirAll(dir, dirPerms); err != nil {
			tb.Fatal(err)
		}

		for j := range filesPerDir {
			if err := os.WriteFile(filepath.Join(dir, "f"+strconv.Itoa(j)+".tf"), small, perms); err != nil {
				tb.Fatal(err)
			}
		}
	}

	blob := make([]byte, blobSize)

	for i := range blobs {
		if err := os.WriteFile(filepath.Join(root, "blob"+strconv.Itoa(i)+".bin"), blob, perms); err != nil {
			tb.Fatal(err)
		}
	}
}

// BenchmarkCopyFolderContentsFast exercises the fast-copy path, whose
// per-file work runs on the workers of [vfs.WalkDirParallel].
func BenchmarkCopyFolderContentsFast(b *testing.B) {
	const (
		kb = 1 << 10
		mb = 1 << 20
	)

	testCases := []struct {
		name        string
		dirs        int
		filesPerDir int
		fileSize    int
		blobs       int
		blobSize    int
	}{
		{name: "small-files", dirs: 400, filesPerDir: 4, fileSize: 2 * kb, blobs: 0, blobSize: 0},
		{name: "mixed", dirs: 200, filesPerDir: 4, fileSize: 2 * kb, blobs: 4, blobSize: 8 * mb},
	}

	l := logger.CreateLogger()

	for _, tc := range testCases {
		b.Run(tc.name, func(b *testing.B) {
			source := b.TempDir()
			buildCopyTree(b, source, tc.dirs, tc.filesPerDir, tc.fileSize, tc.blobs, tc.blobSize)

			dests := b.TempDir()
			fsys := vfs.NewOSFS()

			b.ReportAllocs()

			i := 0

			for b.Loop() {
				dest := filepath.Join(dests, strconv.Itoa(i))
				i++

				if err := util.CopyFolderContents(
					l,
					fsys,
					source,
					dest,
					".terragrunt-bench-manifest",
					util.WithFastCopy(),
				); err != nil {
					b.Fatal(err)
				}

				b.StopTimer()

				if err := os.RemoveAll(dest); err != nil {
					b.Fatal(err)
				}

				b.StartTimer()
			}
		})
	}
}
