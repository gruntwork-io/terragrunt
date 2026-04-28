package cas_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/gruntwork-io/terragrunt/internal/cas"
	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// buildLocalStackFixture lays out a directory tree on disk that mirrors the
// structure used by the remote stack tests so we can exercise the same
// processing pipeline against a local source. The returned path is the
// repo-root; callers append "//stacks/my-stack" to target the stack.
func buildLocalStackFixture(t *testing.T) string {
	t.Helper()

	root := helpers.TmpDirWOSymlinks(t)

	write := func(rel, body string) {
		full := filepath.Join(root, rel)
		require.NoError(t, os.MkdirAll(filepath.Dir(full), 0o755))
		require.NoError(t, os.WriteFile(full, []byte(body), 0o644))
	}

	write("stacks/my-stack/terragrunt.stack.hcl", `unit "service" {
  source = "../..//units/my-service"

  update_source_with_cas = true

  path = "service"
}

unit "plain" {
  source = "../../units/plain-service"
  path   = "plain"
}
`)
	write("units/my-service/terragrunt.hcl", `terraform {
  source = "../..//modules/vpc"

  update_source_with_cas = true
}
`)
	write("units/plain-service/terragrunt.hcl", `terraform {
  source = "../../modules/vpc"
}
`)
	write("modules/vpc/main.tf", `resource "aws_vpc" "main" {
  cidr_block = "10.0.0.0/16"
}
`)
	write("modules/vpc/variables.tf", `variable "name" {
  type = string
}
`)

	return root
}

// snapshotTree reads every regular file under root and returns a sha256 of the
// (relpath, mode, contents) triples in walk order. Used to prove a run didn't
// mutate the source tree, including file permissions.
func snapshotTree(t *testing.T, root string) string {
	t.Helper()

	h := sha256.New()

	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if info.IsDir() || !info.Mode().IsRegular() {
			return nil
		}

		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}

		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}

		h.Write([]byte(rel))
		h.Write([]byte{0})
		h.Write([]byte(info.Mode().String()))
		h.Write([]byte{0})
		h.Write(body)
		h.Write([]byte{0})

		return nil
	})
	require.NoError(t, err)

	return hex.EncodeToString(h.Sum(nil))
}

func TestProcessStackComponent_LocalSource_RewritesStackSources(t *testing.T) {
	t.Parallel()

	root := buildLocalStackFixture(t)
	l := logger.CreateLogger()

	storePath := filepath.Join(helpers.TmpDirWOSymlinks(t), "store")
	c, err := cas.New(cas.WithStorePath(storePath))
	require.NoError(t, err)

	source := root + "//stacks/my-stack"

	result, err := c.ProcessStackComponent(t.Context(), l, source, "stack")
	require.NoError(t, err)

	defer result.Cleanup()

	stackFile := filepath.Join(result.ContentDir, "terragrunt.stack.hcl")
	require.FileExists(t, stackFile)

	content, err := os.ReadFile(stackFile)
	require.NoError(t, err)

	contentStr := string(content)

	assert.Contains(t, contentStr, "cas::sha256:", "service unit source should be rewritten to a SHA-256 CAS ref")
	assert.Contains(t, contentStr, "update_source_with_cas", "flag should be preserved")
	assert.Contains(t, contentStr, `"../../units/plain-service"`, "plain unit source should be unchanged")
}

func TestProcessStackComponent_LocalSource_RewritesUnitSources(t *testing.T) {
	t.Parallel()

	root := buildLocalStackFixture(t)
	l := logger.CreateLogger()

	storePath := filepath.Join(helpers.TmpDirWOSymlinks(t), "store")
	c, err := cas.New(cas.WithStorePath(storePath))
	require.NoError(t, err)

	source := root + "//stacks/my-stack"

	result, err := c.ProcessStackComponent(t.Context(), l, source, "stack")
	require.NoError(t, err)

	defer result.Cleanup()

	// contentDir = <tmp>/repo/stacks/my-stack, so repo root is two dirs up.
	repoCopy := filepath.Dir(filepath.Dir(result.ContentDir))
	unitFile := filepath.Join(repoCopy, "units", "my-service", "terragrunt.hcl")
	require.FileExists(t, unitFile)

	content, err := os.ReadFile(unitFile)
	require.NoError(t, err)

	contentStr := string(content)

	assert.Contains(t, contentStr, "cas::sha256:", "unit terraform source should be rewritten to a SHA-256 CAS ref")
	assert.NotContains(t, contentStr, "modules/vpc", "module path should not appear in the rewritten source")
}

func TestProcessStackComponent_LocalSource_DoesNotMutateInput(t *testing.T) {
	t.Parallel()

	root := buildLocalStackFixture(t)
	l := logger.CreateLogger()

	before := snapshotTree(t, root)

	storePath := filepath.Join(helpers.TmpDirWOSymlinks(t), "store")
	c, err := cas.New(cas.WithStorePath(storePath))
	require.NoError(t, err)

	source := root + "//stacks/my-stack"

	result, err := c.ProcessStackComponent(t.Context(), l, source, "stack")
	require.NoError(t, err)

	result.Cleanup()

	after := snapshotTree(t, root)
	assert.Equal(t, before, after, "processing must not mutate the local source tree")
}

func TestProcessStackComponent_LocalSource_DeterministicOutput(t *testing.T) {
	t.Parallel()

	root := buildLocalStackFixture(t)
	l := logger.CreateLogger()

	readStackFile := func() string {
		storePath := filepath.Join(helpers.TmpDirWOSymlinks(t), "store")
		c, err := cas.New(cas.WithStorePath(storePath))
		require.NoError(t, err)

		source := root + "//stacks/my-stack"

		result, err := c.ProcessStackComponent(t.Context(), l, source, "stack")
		require.NoError(t, err)

		defer result.Cleanup()

		content, err := os.ReadFile(filepath.Join(result.ContentDir, "terragrunt.stack.hcl"))
		require.NoError(t, err)

		return string(content)
	}

	first := readStackFile()
	second := readStackFile()

	assert.Equal(t, first, second, "processing the same local source twice should produce identical output")
}

func TestProcessStackComponent_LocalSource_ContentAddressedCacheKey(t *testing.T) {
	t.Parallel()

	// Two fixtures with the same relative layout but different module contents
	// must yield different synthetic tree hashes — otherwise one source would
	// poison the cache for the other.
	rootA := buildLocalStackFixture(t)
	rootB := buildLocalStackFixture(t)

	// Mutate B's module to make it materially different.
	require.NoError(t, os.WriteFile(
		filepath.Join(rootB, "modules", "vpc", "main.tf"),
		[]byte(`resource "aws_vpc" "different" {
  cidr_block = "192.168.0.0/16"
}
`), 0o644))

	l := logger.CreateLogger()

	runAndExtractServiceRef := func(root string) string {
		storePath := filepath.Join(helpers.TmpDirWOSymlinks(t), "store")
		c, err := cas.New(cas.WithStorePath(storePath))
		require.NoError(t, err)

		source := root + "//stacks/my-stack"

		result, err := c.ProcessStackComponent(t.Context(), l, source, "stack")
		require.NoError(t, err)

		defer result.Cleanup()

		content, err := os.ReadFile(filepath.Join(result.ContentDir, "terragrunt.stack.hcl"))
		require.NoError(t, err)

		blocks, err := cas.ReadStackBlocks(content)
		require.NoError(t, err)

		for _, b := range blocks {
			if b.Name == "service" {
				return b.Source
			}
		}

		t.Fatal("service block not found")

		return ""
	}

	refA := runAndExtractServiceRef(rootA)
	refB := runAndExtractServiceRef(rootB)

	require.True(t, strings.HasPrefix(refA, "cas::sha256:"))
	require.True(t, strings.HasPrefix(refB, "cas::sha256:"))
	assert.NotEqual(t, refA, refB, "different module contents must produce different CAS refs")

	// And identical content at a fresh path must re-hash to the same ref.
	rootC := buildLocalStackFixture(t)
	refC := runAndExtractServiceRef(rootC)
	assert.Equal(t, refA, refC, "identical content at a different absolute path must hash identically")
}

func TestProcessStackComponent_LocalSource_NonExistentPath(t *testing.T) {
	t.Parallel()

	c := newCAS(t)
	l := logger.CreateLogger()

	// Absolute path that does not exist — must not be misinterpreted as a URL.
	source := filepath.Join(helpers.TmpDirWOSymlinks(t), "does-not-exist")

	_, err := c.ProcessStackComponent(t.Context(), l, source, "stack")
	require.Error(t, err, "non-existent local path must fail")
	require.ErrorIs(t, err, fs.ErrNotExist, "error must be a local file-not-found error")
}

func TestProcessStackComponent_LocalSource_RegularFileRejected(t *testing.T) {
	t.Parallel()

	c := newCAS(t)
	l := logger.CreateLogger()

	// A regular file is not a valid component source. The local flow rejects
	// non-directories with ErrNotADirectory; the remote flow would fail at URL
	// parsing / ls-remote. Either way, the call must return an error rather
	// than silently succeeding.
	tmp := helpers.TmpDirWOSymlinks(t)
	filePath := filepath.Join(tmp, "a-file")
	require.NoError(t, os.WriteFile(filePath, []byte("x"), 0o644))

	_, err := c.ProcessStackComponent(t.Context(), l, filePath, "stack")
	require.Error(t, err, "a regular file must not be accepted as a component source")
}

func TestProcessStackComponent_LocalSource_MissingSubdir(t *testing.T) {
	t.Parallel()

	c := newCAS(t)
	l := logger.CreateLogger()

	root := buildLocalStackFixture(t)
	source := root + "//stacks/does-not-exist"

	_, err := c.ProcessStackComponent(t.Context(), l, source, "stack")
	require.Error(t, err, "missing subdir inside a local source must fail")
	assert.Contains(t, err.Error(), "does-not-exist")
}

// TestProcessStackComponent_EmptySourceFails exercises the empty-string
// short-circuit in the local/remote dispatcher. An empty source cannot be a
// valid local directory or a clonable URL, so it must be rejected.
func TestProcessStackComponent_EmptySourceFails(t *testing.T) {
	t.Parallel()

	c := newCAS(t)
	l := logger.CreateLogger()

	_, err := c.ProcessStackComponent(t.Context(), l, "", "stack")
	require.Error(t, err, "empty source must be rejected")
}

// TestProcessStackComponent_GitForcerRoutesRemote confirms that a source with
// a "git::" forcer is treated as remote even though the rest of the string
// could parse as a path. The in-process test server stands in for a real
// remote, so the full remote flow runs end-to-end and must succeed.
func TestProcessStackComponent_GitForcerRoutesRemote(t *testing.T) {
	t.Parallel()

	repoURL := startStackTestServer(t)
	l := logger.CreateLogger()

	storePath := filepath.Join(helpers.TmpDirWOSymlinks(t), "store")
	c, err := cas.New(cas.WithStorePath(storePath), cas.WithCloneDepth(-1))
	require.NoError(t, err)

	source := "git::" + repoURL + "//stacks/my-stack?ref=main"

	result, err := c.ProcessStackComponent(t.Context(), l, source, "stack")
	require.NoError(t, err, "git:: forcer must route through the remote flow and succeed against the test server")

	defer result.Cleanup()

	assert.DirExists(t, result.ContentDir)
}

// TestProcessStackComponent_SSHShorthandRoutesRemote confirms that SSH
// shorthand (git@host:path) is treated as remote, not local. The test runs
// inside a synctest bubble so the context deadline fires on the synthetic
// clock the moment every bubbled goroutine is idle — no real-time wait, no
// dependency on DNS or network behavior. All we care about here is which
// branch of the dispatcher the source was routed through; we assert the
// error originated in the remote pipeline, not the local one.
func TestProcessStackComponent_SSHShorthandRoutesRemote(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		c := newCAS(t)
		l := logger.CreateLogger()

		ctx, cancel := context.WithTimeout(t.Context(), time.Millisecond)
		defer cancel()

		_, err := c.ProcessStackComponent(ctx, l, "git@unreachable.invalid:owner/repo.git", "stack")
		require.Error(t, err, "SSH shorthand must route through the remote flow and fail there")
		assert.NotContains(t, err.Error(), "local source", "error must come from the remote pipeline")
	})
}

func TestProcessStackComponent_LocalSource_MaterializeSynthTree(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	root := buildLocalStackFixture(t)
	l := logger.CreateLogger()

	storePath := filepath.Join(helpers.TmpDirWOSymlinks(t), "store")
	c, err := cas.New(cas.WithStorePath(storePath))
	require.NoError(t, err)

	source := root + "//stacks/my-stack"

	result, err := c.ProcessStackComponent(ctx, l, source, "stack")
	require.NoError(t, err)

	defer result.Cleanup()

	stackContent, err := os.ReadFile(filepath.Join(result.ContentDir, "terragrunt.stack.hcl"))
	require.NoError(t, err)

	blocks, err := cas.ReadStackBlocks(stackContent)
	require.NoError(t, err)

	var serviceSource string

	for _, b := range blocks {
		if b.Name == "service" {
			serviceSource = b.Source

			break
		}
	}

	require.NotEmpty(t, serviceSource)

	trimmed := strings.TrimPrefix(serviceSource, "cas::")
	hash, err := cas.ParseCASRef(trimmed)
	require.NoError(t, err)

	destDir := helpers.TmpDirWOSymlinks(t)
	require.NoError(t, c.MaterializeTree(ctx, l, hash, destDir))

	assert.FileExists(t, filepath.Join(destDir, "terragrunt.hcl"))
}
