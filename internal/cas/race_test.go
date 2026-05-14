// Tests specific to race conditions are verified here

package cas_test

import (
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/cas"
	"github.com/gruntwork-io/terragrunt/internal/getter"
	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCASGetterGetWithRacing(t *testing.T) {
	t.Parallel()

	repoURL := startTestServer(t)

	tempDir := helpers.TmpDirWOSymlinks(t)
	storePath := filepath.Join(tempDir, "store")

	c, err := cas.New(cas.WithStorePath(storePath))
	require.NoError(t, err)

	opts := &cas.CloneOptions{
		Depth: -1,
	}

	l := logger.CreateLogger()

	g := getter.NewCASGetter(l, c, opts)
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

	const workers = 4

	results := make([]string, workers)
	errs := make([]error, workers)

	var wg sync.WaitGroup

	for i := range workers {
		wg.Add(1)

		go func(idx int) {
			defer wg.Done()

			source := root + "//stacks/my-stack"

			result, runErr := c.ProcessStackComponent(t.Context(), l, source, "stack")
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
		assert.Equal(t, results[0], results[i], "all concurrent runs must produce identical rewritten output")
	}
}
