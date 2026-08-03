// Tests specific to race conditions are verified here

package cas_test

import (
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gruntwork-io/terragrunt/internal/cas"
	"github.com/gruntwork-io/terragrunt/internal/getter"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
)

func TestCASGetterGetWithRacing(t *testing.T) {
	t.Parallel()

	repoURL := startTestServer(t)

	tempDir := helpers.TmpDirWOSymlinks(t)
	storePath := filepath.Join(tempDir, "store")

	c, err := cas.New(cas.WithStorePath(storePath))
	require.NoError(t, err)

	v := venv.OSVenv()

	opts := &cas.CloneOptions{
		Depth: -1,
	}

	l := logger.CreateLogger()

	g := getter.NewCASGetter(l, c, v, opts)
	client := getter.Client{
		Getters: []getter.Getter{g},
	}

	tests := []struct {
		name string
		url  string
	}{
		{
			name: "clone via getter with ref",
			url:  "git::" + repoURL + "?ref=main",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			tmpDir := helpers.TmpDirWOSymlinks(t)

			res, err := client.Get(
				t.Context(),
				&getter.Request{
					Src: tt.url,
					Dst: tmpDir,
				},
			)
			require.NoError(t, err)

			assert.Equal(t, tmpDir, res.Dst)
		})
	}
}

func TestProcessStackComponentLocalSourceConcurrentWithRacing(t *testing.T) {
	t.Parallel()

	// Two concurrent ProcessStackComponent calls against the same local source
	// and the same CAS store must succeed without racing on blob/tree writes,
	// and must produce identical rewritten stack files.
	root := buildLocalStackFixture(t)
	l := logger.CreateLogger()

	storePath := filepath.Join(helpers.TmpDirWOSymlinks(t), "store")
	c, err := cas.New(cas.WithStorePath(storePath))
	require.NoError(t, err)

	v := venv.OSVenv()

	const workers = 4

	results := make([]string, workers)
	errs := make([]error, workers)

	var wg sync.WaitGroup

	for i := range workers {
		wg.Add(1)

		go func(idx int) {
			defer wg.Done()

			source := root + "//stacks/my-stack"

			result, runErr := c.ProcessStackComponent(t.Context(), l, v, source, "stack")
			if runErr != nil {
				errs[idx] = runErr
				return
			}

			defer result.Cleanup()

			body, readErr := os.ReadFile(filepath.Join(result.ContentDir, "terragrunt.stack.hcl"))
			if readErr != nil {
				errs[idx] = readErr
				return
			}

			results[idx] = string(body)
		}(i)
	}

	wg.Wait()

	for i, e := range errs {
		require.NoErrorf(t, e, "worker %d failed", i)
	}

	for i := 1; i < workers; i++ {
		assert.Equal(
			t,
			results[0],
			results[i],
			"all concurrent runs must produce identical rewritten output",
		)
	}
}

// TestContentLinkConcurrentSameTargetWithRacing holds Link to its
// concurrency-safety contract: linking the same blob to the same target from
// several goroutines must not fail. A fixed read-only "<target>.tmp" broke
// this, since the race losers opened the winner's 0o444 temp for write.
func TestContentLinkConcurrentSameTargetWithRacing(t *testing.T) {
	t.Parallel()

	l := logger.CreateLogger()

	v := venv.OSVenv()

	storeDir := t.TempDir()
	store := cas.NewStore(storeDir)
	content := cas.NewContent(store)

	const hash = "3333333333333333333333333333333333333333"
	require.NoError(t, content.Store(l, v, hash, []byte("ref: refs/heads/master\n")))

	// Leave the stored blob writable so its perms differ from the requested
	// read-only result, forcing Link onto the copy path even on a
	// same-device filesystem where the hard link would otherwise succeed.
	require.NoError(t, os.Chmod(filepath.Join(storeDir, hash[:2], hash), 0o644))

	targetDir := t.TempDir()
	gitDir := filepath.Join(targetDir, ".git")
	require.NoError(t, os.MkdirAll(gitDir, 0o755))
	targetPath := filepath.Join(gitDir, "HEAD")

	const workers = 8

	errs := make([]error, workers)

	var wg sync.WaitGroup

	for i := range workers {
		wg.Add(1)

		go func(idx int) {
			defer wg.Done()

			errs[idx] = content.Link(t.Context(), v, hash, targetPath, 0o644)
		}(i)
	}

	wg.Wait()

	for i, e := range errs {
		require.NoErrorf(t, e, "worker %d failed to link the shared target", i)
	}
}
