package cas_test

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/gruntwork-io/terragrunt/internal/cas"
	"github.com/gruntwork-io/terragrunt/internal/git"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"
)

// Sizes of the fixtures the store-hygiene benchmarks run against. The
// ingest repository is wide enough that per-object bookkeeping in the
// store shows up as a file count rather than as noise, and the pinned
// history is deep enough that fetching one commit and fetching every
// commit are plainly different transfers.
const (
	benchIngestFiles       = 1000
	benchPinnedFiles       = 20
	benchPinnedCommits     = 500
	benchPinnedCommitDepth = 100
)

// benchLogger returns the test logger with its output discarded, so a
// debug line written mid-run cannot land inside the benchmark output a
// tool has to parse.
func benchLogger() log.Logger {
	l := logger.CreateLogger()
	l.SetOptions(log.WithOutput(io.Discard))

	return l
}

// benchRepoFiles builds count generated OpenTofu files spread over a
// hundred directories, so an ingested tree has both breadth and depth.
func benchRepoFiles(count int) map[string][]byte {
	files := make(map[string][]byte, count)

	for i := range count {
		path := fmt.Sprintf("modules/mod%03d/file%04d.tf", i%100, i)
		files[path] = fmt.Appendf(nil, "resource \"null_resource\" \"r%d\" {}\n", i)
	}

	return files
}

// sharedIngestBenchServer serves the wide repository behind
// BenchmarkColdIngestStore, built once for the whole process so a
// -count run pays for it once.
var sharedIngestBenchServer = sync.OnceValues(newIngestBenchServer)

func newIngestBenchServer() (*git.Server, error) {
	return newSharedFixtureServer(benchRepoFiles(benchIngestFiles), "add ingest fixture")
}

// BenchmarkColdIngestStore times a clone that has to ingest every blob
// and tree, and reports how many regular files the resulting store
// holds outside its bare git repositories.
func BenchmarkColdIngestStore(b *testing.B) {
	srv, err := sharedIngestBenchServer()
	require.NoError(b, err)

	repoURL := uniqueRepoURL(srv)
	l := benchLogger()
	v := venvtest.NewOSWithEmptyEnv()
	tempDir := b.TempDir()

	var lastStore string

	for i := 0; b.Loop(); i++ {
		b.StopTimer()

		storePath := filepath.Join(tempDir, "store", strconv.Itoa(i))
		targetPath := filepath.Join(tempDir, "repo", strconv.Itoa(i))
		lastStore = storePath

		c, err := cas.New(venvtest.NewWithOSFS(), cas.WithStorePath(storePath))
		require.NoError(b, err)

		b.StartTimer()

		require.NoError(b, c.Clone(b.Context(), l, v, repoURL,
			cas.WithDir(targetPath), cas.WithDepth(-1)))
	}

	b.ReportMetric(float64(countStoreObjectFiles(b, lastStore)), "store-files/op")
}

// pinnedBenchFixture is the deep-history repository behind
// BenchmarkPinnedSHAFetch, along with the full object name of the
// commit the benchmark pins.
type pinnedBenchFixture struct {
	url string
	sha string
}

// sharedPinnedBenchFixture builds the deep-history repository once for
// the whole process; five hundred commits cost a git spawn each.
var sharedPinnedBenchFixture = sync.OnceValues(newPinnedBenchFixture)

func newPinnedBenchFixture() (_ *pinnedBenchFixture, retErr error) {
	srv, err := git.NewServer()
	if err != nil {
		return nil, err
	}

	defer func() {
		if retErr != nil {
			retErr = errorsJoinClose(retErr, srv)
		}
	}()

	ctx := context.Background()

	if err := srv.CommitFiles(ctx, benchRepoFiles(benchPinnedFiles), "add pinned fixture"); err != nil {
		return nil, err
	}

	var pinned string

	for i := range benchPinnedCommits {
		if err := srv.CommitEmpty(ctx, "filler"); err != nil {
			return nil, err
		}

		if i == benchPinnedCommits-benchPinnedCommitDepth-1 {
			if pinned, err = srv.Head(ctx); err != nil {
				return nil, err
			}
		}
	}

	if _, err := srv.Start(ctx); err != nil {
		return nil, err
	}

	registerSharedServer(srv)

	return &pinnedBenchFixture{url: uniqueRepoURL(srv), sha: pinned}, nil
}

// errorsJoinClose closes srv and folds any shutdown failure into err.
func errorsJoinClose(err error, srv *git.Server) error {
	if closeErr := srv.Close(); closeErr != nil {
		return fmt.Errorf("%w (closing server: %w)", err, closeErr)
	}

	return err
}

// BenchmarkPinnedSHAFetch times a clone of a commit named by full
// object name, well behind the tip of a five-hundred-commit history,
// and reports how many git objects the bare repository the CAS git
// store built for it ends up holding.
func BenchmarkPinnedSHAFetch(b *testing.B) {
	fixture, err := sharedPinnedBenchFixture()
	require.NoError(b, err)

	l := benchLogger()
	v := venvtest.NewOSWithEmptyEnv()
	tempDir := b.TempDir()

	var lastStore string

	for i := 0; b.Loop(); i++ {
		b.StopTimer()

		storePath := filepath.Join(tempDir, "store", strconv.Itoa(i))
		targetPath := filepath.Join(tempDir, "repo", strconv.Itoa(i))
		lastStore = storePath

		c, err := cas.New(venvtest.NewWithOSFS(), cas.WithStorePath(storePath))
		require.NoError(b, err)

		b.StartTimer()

		require.NoError(b, c.Clone(b.Context(), l, v, fixture.url,
			cas.WithDir(targetPath), cas.WithBranch(fixture.sha), cas.WithDepth(1)))
	}

	b.ReportMetric(float64(countBareRepoObjects(b, lastStore)), "objects/op")
}

// countStoreObjectFiles counts the regular files under storePath,
// skipping the bare git repositories, so the result covers the object
// partitions and whatever per-object bookkeeping sits beside them.
func countStoreObjectFiles(b *testing.B, storePath string) int {
	b.Helper()

	gitRoot := filepath.Join(storePath, "git")
	count := 0

	err := filepath.WalkDir(storePath, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if d.IsDir() {
			if path == gitRoot {
				return fs.SkipDir
			}

			return nil
		}

		if d.Type().IsRegular() {
			count++
		}

		return nil
	})
	require.NoError(b, err)

	return count
}

// countBareRepoObjects returns the number of objects, loose and packed,
// in the single bare repository the CAS git store under storePath
// holds.
func countBareRepoObjects(b *testing.B, storePath string) int {
	b.Helper()

	gitRoot := filepath.Join(storePath, "git")

	entries, err := os.ReadDir(gitRoot)
	require.NoError(b, err)
	require.Len(b, entries, 1, "expected exactly one bare repository in the git store")

	bare := filepath.Join(gitRoot, entries[0].Name(), "repo")

	out, err := exec.CommandContext(b.Context(), "git", "-C", bare, "count-objects", "-v").Output()
	require.NoError(b, err)

	return parseCountObjects(b, string(out))
}

// parseCountObjects sums the loose and packed object counts reported by
// `git count-objects -v`.
func parseCountObjects(b *testing.B, out string) int {
	b.Helper()

	total := 0

	for line := range strings.SplitSeq(out, "\n") {
		field, value, ok := strings.Cut(strings.TrimSpace(line), ": ")
		if !ok || (field != "count" && field != "in-pack") {
			continue
		}

		n, err := strconv.Atoi(value)
		require.NoError(b, err)

		total += n
	}

	return total
}
