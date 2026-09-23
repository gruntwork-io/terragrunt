package cas_test

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/gruntwork-io/terragrunt/internal/cas"
	"github.com/gruntwork-io/terragrunt/internal/git"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"
)

// Shape of the synthetic repository BenchmarkLinkTree materializes: 400 files
// across 39 directories nested three deep.
const (
	linkTreeBenchDepth  = 3
	linkTreeBenchBranch = 3
	linkTreeBenchFiles  = 10
)

// linkTreeBenchFixture stores a synthetic tree and returns the hash of its
// root. A single counter numbers blobs and trees, so no two share a hash.
type linkTreeBenchFixture struct {
	content *cas.Content
	hashes  int
}

func (f *linkTreeBenchFixture) nextHash() string {
	f.hashes++

	return fmt.Sprintf("%010d", f.hashes)
}

func (f *linkTreeBenchFixture) store(b *testing.B, l log.Logger, v *venv.Venv, data []byte) string {
	b.Helper()

	hash := f.nextHash()
	require.NoError(b, f.content.Store(l, v, hash, data, cas.StoredFilePerms))

	return hash
}

func (f *linkTreeBenchFixture) tree(b *testing.B, l log.Logger, v *venv.Venv, depth int) string {
	b.Helper()

	entries := make([]string, 0, linkTreeBenchFiles+linkTreeBenchBranch)

	for i := range linkTreeBenchFiles {
		blob := f.store(b, l, v, []byte(strings.Repeat("x", 512)))
		entries = append(entries, fmt.Sprintf("100644 blob %s file%d", blob, i))
	}

	if depth > 0 {
		for i := range linkTreeBenchBranch {
			sub := f.tree(b, l, v, depth-1)
			entries = append(entries, fmt.Sprintf("040000 tree %s dir%d", sub, i))
		}
	}

	return f.store(b, l, v, []byte(strings.Join(entries, "\n")))
}

// BenchmarkLinkTree materializes a nested tree out of a store that already
// holds every blob, so the timed region is materialization alone.
func BenchmarkLinkTree(b *testing.B) {
	l := logger.CreateLogger()
	v := venvtest.NewWithOSFS()

	tempDir := b.TempDir()
	storePath := filepath.Join(tempDir, "store")

	require.NoError(b, v.FS.MkdirAll(storePath, cas.DefaultDirPerms))

	store := cas.NewStore(storePath)
	fixture := &linkTreeBenchFixture{content: cas.NewContent(store)}

	rootHash := fixture.tree(b, l, v, linkTreeBenchDepth)

	data, err := fixture.content.Read(v, rootHash)
	require.NoError(b, err)

	tree, err := git.ParseTree(data, "bench")
	require.NoError(b, err)

	i := 0

	for b.Loop() {
		target := filepath.Join(tempDir, "target", strconv.Itoa(i))
		require.NoError(b, v.FS.MkdirAll(target, cas.DefaultDirPerms))
		require.NoError(b, cas.LinkTree(b.Context(), l, v, store, store, tree, target))

		i++
	}
}
