package catalog

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terragrunt/options"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUpdateConfigPath(t *testing.T) {
	t.Parallel()

	curDir, err := os.Getwd()
	require.NoError(t, err)

	basePath := filepath.Join(curDir, "testdata/fixture-find-config-file")

	testCases := []struct {
		path         string
		expectedPath string
	}{
		{
			filepath.Join(basePath, "dir1/dir2/dir3/file1"),
			filepath.Join(basePath, "file1"),
		},
		{
			filepath.Join(basePath, "dir1/dir2/dir3/file2"),
			filepath.Join(basePath, "dir1/file2"),
		},
		{
			filepath.Join(basePath, "dir1/dir2/dir3/file4"),
			filepath.Join(basePath, "dir1/dir2/dir3/file4"),
		},
		{
			filepath.Join(basePath, "dir1/dir2/dir3/does-no-exist"),
			"",
		},
	}

	for i, testCase := range testCases {
		testCase := testCase

		t.Run(fmt.Sprintf("testCase-%d", i), func(t *testing.T) {
			t.Parallel()

			opts, err := options.NewTerragruntOptionsWithConfigPath(testCase.path)
			require.NoError(t, err)

			err = updateConfigPath(opts)
			if testCase.expectedPath == "" {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, testCase.expectedPath, opts.TerragruntConfigPath)
		})
	}
}
