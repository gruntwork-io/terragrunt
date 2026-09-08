package cas_test

import (
	"fmt"
	"path/filepath"
	"strconv"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/gruntwork-io/terragrunt/internal/cas"
	"github.com/gruntwork-io/terragrunt/internal/git"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"
)

// The trees the link-mode benchmarks materialize.
//
// benchModuleSource is sized from representative catalog repositories: a
// few hundred files at a median around 2 KiB, with a tail into the hundreds
// of kilobytes. It holds 500 files totalling 7.3 MiB, at a 2 KiB median and a
// 15 KiB mean. A mode charging per file and a mode charging per byte
// separate on that tail, so benchVendoredArtifacts carries the build
// artifacts a module ships beside its configuration, where the bytes
// dominate.
var (
	benchModuleSource = benchTreeShape{
		name: "module-source",
		buckets: []benchFileBucket{
			{count: 350, size: 2 * 1024},
			{count: 100, size: 13 * 1024},
			{count: 45, size: 64 * 1024},
			{count: 5, size: 512 * 1024},
		},
	}

	benchVendoredArtifacts = benchTreeShape{
		name: "vendored-artifacts",
		buckets: []benchFileBucket{
			{count: 100, size: 2 * 1024},
			{count: 20, size: 2 * 1024 * 1024},
		},
	}
)

// benchTreeShape describes a fixture repository as buckets of equally sized
// files, so a fixture can carry the spread of sizes a real source has rather
// than one size repeated.
type benchTreeShape struct {
	name    string
	buckets []benchFileBucket
}

// benchFileBucket is count files of size bytes each.
type benchFileBucket struct {
	count int
	size  int
}

// files returns how many files the shape holds.
func (s benchTreeShape) files() int {
	total := 0
	for _, bucket := range s.buckets {
		total += bucket.count
	}

	return total
}

// The repositories behind the link-mode benchmarks, each built once for the
// whole process.
var (
	sharedModuleSourceServer = sync.OnceValues(func() (*git.Server, error) {
		return newSharedFixtureServer(benchShapeFiles(benchModuleSource), "add module source fixture")
	})
	sharedVendoredArtifactsServer = sync.OnceValues(func() (*git.Server, error) {
		return newSharedFixtureServer(
			benchShapeFiles(benchVendoredArtifacts),
			"add vendored artifacts fixture",
		)
	})
)

// benchShapes pairs each shape with the fixture server serving it.
var benchShapes = []struct {
	server func() (*git.Server, error)
	shape  benchTreeShape
}{
	{shape: benchModuleSource, server: sharedModuleSourceServer},
	{shape: benchVendoredArtifacts, server: sharedVendoredArtifactsServer},
}

// benchShapeFiles builds the fixture files for shape, spread over a hundred
// directories so an ingested tree has breadth and depth. The bodies are a
// repeating pattern rather than random bytes, which keeps the fixture cheap
// to commit without changing what the benchmarks measure: the store holds
// content uncompressed, so a copy moves the whole file either way.
func benchShapeFiles(shape benchTreeShape) map[string][]byte {
	files := make(map[string][]byte, shape.files())
	i := 0

	for _, bucket := range shape.buckets {
		body := make([]byte, bucket.size)
		for n := range body {
			body[n] = byte('a' + n%26)
		}

		for range bucket.count {
			files[fmt.Sprintf("modules/mod%03d/file%04d.tf", i%100, i)] = body
			i++
		}
	}

	return files
}

// BenchmarkLinkModes times materializing one pre-ingested tree into a fresh
// directory under each mode a caller can ask for. The clone sub-benchmark
// reports whether the filesystem holding the target offered a copy-on-write
// clone, since without one the mode degrades to a hard link.
func BenchmarkLinkModes(b *testing.B) {
	benchmarkLinkModes(b, []cas.LinkMode{cas.LinkModeHardlink, cas.LinkModeClone, cas.LinkModeCopy})
}

// BenchmarkMutableLinkModes times materializing a tree the caller intends to
// edit under the two modes that can serve one. A hard link is not among
// them, since it hands out the store's own file; [resolveLinkMode] turns
// such a request into the clone measured here, and the copy is what that
// clone degrades to on a filesystem with no copy-on-write support.
func BenchmarkMutableLinkModes(b *testing.B) {
	benchmarkLinkModes(b, []cas.LinkMode{cas.LinkModeCopy, cas.LinkModeClone}, cas.WithForceCopy())
}

// benchmarkLinkModes materializes every shape in [benchShapes] under each of
// modes, passing treeOpts to every LinkTree call.
func benchmarkLinkModes(b *testing.B, modes []cas.LinkMode, treeOpts ...cas.LinkTreeOption) {
	b.Helper()

	for _, tt := range benchShapes {
		b.Run(tt.shape.name, func(b *testing.B) {
			srv, err := tt.server()
			require.NoError(b, err)

			benchmarkShapeLinkModes(b, srv, tt.shape, modes, treeOpts)
		})
	}
}

// benchmarkShapeLinkModes ingests the tree srv serves, then materializes it
// under each mode.
func benchmarkShapeLinkModes(
	b *testing.B,
	srv *git.Server,
	shape benchTreeShape,
	modes []cas.LinkMode,
	treeOpts []cas.LinkTreeOption,
) {
	b.Helper()

	repoURL := uniqueRepoURL(srv)
	l := benchLogger()
	v := venvtest.NewOSWithEmptyEnv()
	root := b.TempDir()

	c, err := cas.New(venvtest.NewWithOSFS(), cas.WithStorePath(filepath.Join(root, "store")))
	require.NoError(b, err)

	require.NoError(b, c.Clone(b.Context(), l, v, repoURL,
		cas.WithDir(filepath.Join(root, "warm")), cas.WithDepth(-1)))

	tree := benchLinkTree(b, v, repoURL, filepath.Join(root, "src"), shape.files())
	cloneFallback := cloneFallbackMetric(b, v, root)

	for _, mode := range modes {
		b.Run(mode.String(), func(b *testing.B) {
			targets := filepath.Join(root, "target", mode.String())
			opts := append([]cas.LinkTreeOption{cas.WithTreeLinkMode(mode)}, treeOpts...)

			for i := 0; b.Loop(); i++ {
				b.StopTimer()

				if i > 0 {
					require.NoError(b, v.FS.RemoveAll(filepath.Join(targets, strconv.Itoa(i-1))))
				}

				dir := filepath.Join(targets, strconv.Itoa(i))

				b.StartTimer()

				require.NoError(b, cas.LinkTree(b.Context(), l, v, c.BlobStore(), c.TreeStore(),
					tree, dir, opts...))
			}

			if mode == cas.LinkModeClone {
				b.ReportMetric(cloneFallback, "clone-fallback")
			}
		})
	}
}

// benchLinkTree clones repoURL into dir with git and reads its flat tree, so
// LinkTree runs against the same blobs the CAS store ingested. It checks the
// tree holds wantEntries, which catches a fixture that did not commit what
// the benchmark sizes itself against.
func benchLinkTree(b *testing.B, v *venv.Venv, repoURL, dir string, wantEntries int) *git.Tree {
	b.Helper()

	require.NoError(b, v.FS.MkdirAll(dir, 0o755))

	runner, err := git.NewGitRunner(v)
	require.NoError(b, err)

	runner = runner.WithWorkDir(dir)
	require.NoError(b, runner.Clone(b.Context(), repoURL, false, 0, ""))

	tree, err := runner.LsTreeRecursive(b.Context(), "HEAD")
	require.NoError(b, err)
	require.Len(b, tree.Entries(), wantEntries)

	return tree
}

// cloneFallbackMetric reports 1 when the filesystem under dir has no
// copy-on-write clone, which is when [cas.LinkModeClone] falls back to
// hard linking or copying, and 0 when the clone is served as asked.
func cloneFallbackMetric(b *testing.B, v *venv.Venv, dir string) float64 {
	b.Helper()

	src := filepath.Join(dir, "clone-probe-src")
	require.NoError(b, vfs.WriteFile(v.FS, src, []byte("probe"), 0o644))

	if err := vfs.CloneFile(v.FS, src, filepath.Join(dir, "clone-probe-dst")); err != nil {
		return 1
	}

	return 0
}
