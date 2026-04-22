package cas_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/cas"
	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSplitSourceDoubleSlash(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		source     string
		wantBase   string
		wantSubdir string
	}{
		{
			name:       "with double slash",
			source:     "../..//modules/ec2-asg-service",
			wantBase:   "../..",
			wantSubdir: "modules/ec2-asg-service",
		},
		{
			name:       "without double slash",
			source:     "../../modules/ec2-asg-service",
			wantBase:   "../../modules/ec2-asg-service",
			wantSubdir: "",
		},
		{
			name:       "double slash at start",
			source:     "//modules/vpc",
			wantBase:   "",
			wantSubdir: "modules/vpc",
		},
		{
			name:       "only path",
			source:     "../units/service",
			wantBase:   "../units/service",
			wantSubdir: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			base, subdir := cas.SplitSourceDoubleSlash(tt.source)
			assert.Equal(t, tt.wantBase, base)
			assert.Equal(t, tt.wantSubdir, subdir)
		})
	}
}

func TestResolveInRepoSource(t *testing.T) {
	t.Parallel()

	repoRoot := filepath.Join(string(filepath.Separator), "tmp", "repo")
	dirPath := filepath.Join(repoRoot, "stacks", "app")

	tests := []struct {
		wantErr error
		name    string
		source  string
		want    string
	}{
		{
			name:   "sibling path within repo",
			source: "../../units/service",
			want:   filepath.Join(repoRoot, "units", "service"),
		},
		{
			name:   "double-slash subdir within repo",
			source: "../..//modules/ec2",
			want:   filepath.Join(repoRoot, "modules", "ec2"),
		},
		{
			name:   "nested path within dirPath",
			source: "child",
			want:   filepath.Join(dirPath, "child"),
		},
		{
			name:    "absolute source rejected",
			source:  filepath.Join(string(filepath.Separator), "etc", "passwd"),
			wantErr: cas.ErrAbsoluteSource,
		},
		{
			name:    "parent escape rejected",
			source:  "../../../../etc",
			wantErr: cas.ErrSourceEscapesRepo,
		},
		{
			name:    "escape via double-slash subdir",
			source:  "..//../../../etc",
			wantErr: cas.ErrSourceEscapesRepo,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := cas.ResolveInRepoSource(repoRoot, dirPath, tt.source)
			if tt.wantErr != nil {
				assert.ErrorIs(t, err, tt.wantErr)

				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestDeterministicTreeHash(t *testing.T) {
	t.Parallel()

	// SHA-1 length refHash (40 chars) → produces SHA-1 output (40 chars)
	sha1Ref := "f39ea0ebf891c9954c89d07b73b487ff938ef08b"
	hash1 := cas.DeterministicTreeHash(sha1Ref, "stacks/ec2-asg-stateful-service")
	hash2 := cas.DeterministicTreeHash(sha1Ref, "stacks/ec2-asg-stateful-service")
	assert.Equal(t, hash1, hash2, "same inputs must produce the same hash")
	assert.Len(t, hash1, 40, "SHA-1 refHash should produce 40-char output")

	hash3 := cas.DeterministicTreeHash(sha1Ref, "stacks/different")
	assert.NotEqual(t, hash1, hash3)

	hash4 := cas.DeterministicTreeHash("0000000000000000000000000000000000000000", "stacks/ec2-asg-stateful-service")
	assert.NotEqual(t, hash1, hash4)

	// SHA-256 length refHash (64 chars) → produces SHA-256 output (64 chars)
	sha256Ref := "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
	hash5 := cas.DeterministicTreeHash(sha256Ref, "stacks/ec2-asg-stateful-service")
	assert.Len(t, hash5, 64, "SHA-256 refHash should produce 64-char output")
}

func TestProcessStackComponent_RewritesStackSources(t *testing.T) {
	t.Parallel()

	repoURL := startStackTestServer(t)
	l := logger.CreateLogger()

	storePath := filepath.Join(helpers.TmpDirWOSymlinks(t), "store")
	c, err := cas.New(cas.WithStorePath(storePath), cas.WithCloneDepth(-1))
	require.NoError(t, err)

	// Source mimics what a stack generates: <repo-url>//<subdir>?ref=<branch>
	source := repoURL + "//stacks/my-stack?ref=main"

	result, err := c.ProcessStackComponent(t.Context(), l, source, "stack")
	require.NoError(t, err)

	defer result.Cleanup()

	// The content dir should contain the rewritten terragrunt.stack.hcl
	stackFile := filepath.Join(result.ContentDir, "terragrunt.stack.hcl")
	require.FileExists(t, stackFile)

	content, err := os.ReadFile(stackFile)
	require.NoError(t, err)

	contentStr := string(content)

	// The "service" unit had update_source_with_cas = true, so its source should
	// be rewritten to a cas:: reference.
	assert.Contains(t, contentStr, "cas::", "service unit source should be rewritten to CAS ref")
	assert.Contains(t, contentStr, "update_source_with_cas", "flag should be preserved")

	// The "plain" unit had no update_source_with_cas, so its source must remain unchanged.
	assert.Contains(t, contentStr, `"../../units/plain-service"`, "plain unit source should be unchanged")
}

func TestProcessStackComponent_RewritesUnitSources(t *testing.T) {
	t.Parallel()

	repoURL := startStackTestServer(t)
	l := logger.CreateLogger()

	storePath := filepath.Join(helpers.TmpDirWOSymlinks(t), "store")
	c, err := cas.New(cas.WithStorePath(storePath), cas.WithCloneDepth(-1))
	require.NoError(t, err)

	source := repoURL + "//stacks/my-stack?ref=main"

	result, err := c.ProcessStackComponent(t.Context(), l, source, "stack")
	require.NoError(t, err)

	defer result.Cleanup()

	// The unit that was recursively processed should have its terraform.source rewritten.
	// ProcessStackComponent processes stacks/my-stack, which references units/my-service.
	// The processDirectory call should reach units/my-service/terragrunt.hcl and rewrite it.

	// First, resolve the path to the cloned unit file.
	// The contentDir is <tempDir>/repo/stacks/my-stack, and the unit is at
	// <tempDir>/repo/units/my-service/terragrunt.hcl relative to repo root.
	repoRoot := filepath.Dir(filepath.Dir(result.ContentDir))
	unitFile := filepath.Join(repoRoot, "units", "my-service", "terragrunt.hcl")
	require.FileExists(t, unitFile)

	content, err := os.ReadFile(unitFile)
	require.NoError(t, err)

	contentStr := string(content)

	assert.Contains(t, contentStr, "cas::", "unit terraform source should be rewritten to CAS ref")
	assert.Contains(t, contentStr, "sha1:", "CAS ref should name the hash algorithm")
	assert.NotContains(
		t,
		contentStr,
		"modules/vpc",
		"module path should not appear in the cas:: URL when using a synthetic tree",
	)
}

func TestProcessStackComponent_CreatesSyntheticTrees(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	repoURL := startStackTestServer(t)
	l := logger.CreateLogger()

	storePath := filepath.Join(helpers.TmpDirWOSymlinks(t), "store")
	c, err := cas.New(cas.WithStorePath(storePath), cas.WithCloneDepth(-1))
	require.NoError(t, err)

	source := repoURL + "//stacks/my-stack?ref=main"

	result, err := c.ProcessStackComponent(ctx, l, source, "stack")
	require.NoError(t, err)

	defer result.Cleanup()

	// Read the rewritten stack file to extract the CAS ref for the "service" unit.
	stackFile := filepath.Join(result.ContentDir, "terragrunt.stack.hcl")
	content, err := os.ReadFile(stackFile)
	require.NoError(t, err)

	blocks, err := cas.ReadStackBlocks(content)
	require.NoError(t, err)

	var serviceSource string

	for _, b := range blocks {
		if b.Name == "service" {
			serviceSource = b.Source

			break
		}
	}

	require.NotEmpty(t, serviceSource, "should find service block in rewritten stack file")
	assert.True(t, strings.HasPrefix(serviceSource, "cas::"), "source should start with cas:: prefix")

	// Parse the CAS ref to get the hash
	trimmed := strings.TrimPrefix(serviceSource, "cas::")
	hash, err := cas.ParseCASRef(trimmed)
	require.NoError(t, err)

	// The synthetic tree should be stored in the synth store
	synthStore := cas.NewStore(filepath.Join(storePath, "synth", "trees"))
	assert.False(t, synthStore.NeedsWrite(hash), "synthetic tree should exist in synth store")

	// Verify the tree can be read and contains entries
	synthContent := cas.NewContent(synthStore)
	treeData, err := synthContent.Read(hash)
	require.NoError(t, err)
	assert.NotEmpty(t, treeData, "synthetic tree data should not be empty")
}

func TestProcessStackComponent_DeterministicOutput(t *testing.T) {
	t.Parallel()

	repoURL := startStackTestServer(t)
	l := logger.CreateLogger()

	readStackFile := func() string {
		storePath := filepath.Join(helpers.TmpDirWOSymlinks(t), "store")
		c, err := cas.New(cas.WithStorePath(storePath), cas.WithCloneDepth(-1))
		require.NoError(t, err)

		source := repoURL + "//stacks/my-stack?ref=main"

		result, err := c.ProcessStackComponent(t.Context(), l, source, "stack")
		require.NoError(t, err)

		defer result.Cleanup()

		content, err := os.ReadFile(filepath.Join(result.ContentDir, "terragrunt.stack.hcl"))
		require.NoError(t, err)

		return string(content)
	}

	// Process the same source twice with separate CAS stores.
	first := readStackFile()
	second := readStackFile()

	// Both runs should produce identical output — the CAS hashes are
	// deterministic based on ref + path, so regeneration must not produce diffs.
	assert.Equal(t, first, second, "processing the same source twice should produce identical output")
}

func TestProcessStackComponent_MaterializeSynthTree(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	repoURL := startStackTestServer(t)
	l := logger.CreateLogger()

	storePath := filepath.Join(helpers.TmpDirWOSymlinks(t), "store")
	c, err := cas.New(cas.WithStorePath(storePath), cas.WithCloneDepth(-1))
	require.NoError(t, err)

	source := repoURL + "//stacks/my-stack?ref=main"

	result, err := c.ProcessStackComponent(ctx, l, source, "stack")
	require.NoError(t, err)

	defer result.Cleanup()

	// Extract the CAS hash from the rewritten stack file
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

	require.NotEmpty(t, serviceSource, "should find rewritten service source")

	trimmed := strings.TrimPrefix(serviceSource, "cas::")
	hash, err := cas.ParseCASRef(trimmed)
	require.NoError(t, err)

	// Materialize the synthetic tree to a new directory
	destDir := helpers.TmpDirWOSymlinks(t)
	err = c.MaterializeTree(ctx, l, hash, destDir)
	require.NoError(t, err)

	// The materialized tree should contain the unit's terragrunt.hcl
	assert.FileExists(t, filepath.Join(destDir, "terragrunt.hcl"))
}

func TestProcessStackComponent_InvalidRefFails(t *testing.T) {
	t.Parallel()

	repoURL := startStackTestServer(t)
	l := logger.CreateLogger()

	storePath := filepath.Join(helpers.TmpDirWOSymlinks(t), "store")
	c, err := cas.New(cas.WithStorePath(storePath), cas.WithCloneDepth(-1))
	require.NoError(t, err)

	source := repoURL + "//stacks/my-stack?ref=nonexistent-tag"

	_, err = c.ProcessStackComponent(t.Context(), l, source, "stack")
	require.Error(t, err, "should fail when ref does not exist")
}

func TestProcessStackComponent_InvalidSubdirFails(t *testing.T) {
	t.Parallel()

	repoURL := startStackTestServer(t)
	l := logger.CreateLogger()

	storePath := filepath.Join(helpers.TmpDirWOSymlinks(t), "store")
	c, err := cas.New(cas.WithStorePath(storePath), cas.WithCloneDepth(-1))
	require.NoError(t, err)

	source := repoURL + "//nonexistent/path?ref=main"

	_, err = c.ProcessStackComponent(t.Context(), l, source, "stack")
	require.Error(t, err, "should fail when subdir does not exist")
}

func TestProcessStackComponent_BlobsStoredInCAS(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	repoURL := startStackTestServer(t)
	l := logger.CreateLogger()

	storePath := filepath.Join(helpers.TmpDirWOSymlinks(t), "store")
	c, err := cas.New(cas.WithStorePath(storePath), cas.WithCloneDepth(-1))
	require.NoError(t, err)

	source := repoURL + "//stacks/my-stack?ref=main"

	result, err := c.ProcessStackComponent(ctx, l, source, "stack")
	require.NoError(t, err)

	defer result.Cleanup()

	// Verify that the blob store has content after processing.
	// The CAS should have stored blobs for all files in the repo.
	blobStore := cas.NewStore(filepath.Join(storePath, "blobs"))

	entries, err := os.ReadDir(blobStore.Path())
	require.NoError(t, err)
	assert.NotEmpty(t, entries, "blob store should contain entries after processing")

	// Verify the tree store also has content (the root tree from the clone).
	treeStore := cas.NewStore(filepath.Join(storePath, "trees"))

	entries, err = os.ReadDir(treeStore.Path())
	require.NoError(t, err)
	assert.NotEmpty(t, entries, "tree store should contain entries after processing")
}
