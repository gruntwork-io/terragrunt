package terraform

import (
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gruntwork-io/terragrunt/options"
)

func TestSplitSourceUrl(t *testing.T) {

	t.Parallel()

	testCases := []struct {
		name               string
		sourceUrl          string
		expectedRootRepo   string
		expectedModulePath string
	}{
		{"root-path-only-no-double-slash", "/foo", "/foo", ""},
		{"parent-path-one-child-no-double-slash", "/foo/bar", "/foo/bar", ""},
		{"parent-path-multiple-children-no-double-slash", "/foo/bar/baz/blah", "/foo/bar/baz/blah", ""},
		{"relative-path-no-children-no-double-slash", "../foo", "../foo", ""},
		{"relative-path-one-child-no-double-slash", "../foo/bar", "../foo/bar", ""},
		{"relative-path-multiple-children-no-double-slash", "../foo/bar/baz/blah", "../foo/bar/baz/blah", ""},
		{"root-path-only-with-double-slash", "/foo//", "/foo", ""},
		{"parent-path-one-child-with-double-slash", "/foo//bar", "/foo", "bar"},
		{"parent-path-multiple-children-with-double-slash", "/foo/bar//baz/blah", "/foo/bar", "baz/blah"},
		{"relative-path-no-children-with-double-slash", "..//foo", "..", "foo"},
		{"relative-path-one-child-with-double-slash", "../foo//bar", "../foo", "bar"},
		{"relative-path-multiple-children-with-double-slash", "../foo/bar//baz/blah", "../foo/bar", "baz/blah"},
		{"parent-url-one-child-no-double-slash", "ssh://git@github.com/foo/modules.git/foo", "ssh://git@github.com/foo/modules.git/foo", ""},
		{"parent-url-multiple-children-no-double-slash", "ssh://git@github.com/foo/modules.git/foo/bar/baz/blah", "ssh://git@github.com/foo/modules.git/foo/bar/baz/blah", ""},
		{"parent-url-one-child-with-double-slash", "ssh://git@github.com/foo/modules.git//foo", "ssh://git@github.com/foo/modules.git", "foo"},
		{"parent-url-multiple-children-with-double-slash", "ssh://git@github.com/foo/modules.git//foo/bar/baz/blah", "ssh://git@github.com/foo/modules.git", "foo/bar/baz/blah"},
		{"separate-ref-with-slash", "ssh://git@github.com/foo/modules.git//foo?ref=feature/modules", "ssh://git@github.com/foo/modules.git?ref=feature/modules", "foo"},
	}

	for _, testCase := range testCases {
		// Save a local copy in scope so all the tests don't run the final item in the loop
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			sourceUrl, err := url.Parse(testCase.sourceUrl)
			require.NoError(t, err)

			terragruntOptions, err := options.NewTerragruntOptionsForTest("testing")
			require.NoError(t, err)

			actualRootRepo, actualModulePath, err := splitSourceUrl(sourceUrl, terragruntOptions.Logger)
			require.NoError(t, err)

			assert.Equal(t, testCase.expectedRootRepo, actualRootRepo.String())
			assert.Equal(t, testCase.expectedModulePath, actualModulePath)
		})
	}
}

func TestRegressionSupportForGitRemoteCodecommit(t *testing.T) {
	t.Parallel()

	terragruntOptions, err := options.NewTerragruntOptionsForTest("testing")
	require.NoError(t, err)

	source := "git::codecommit::ap-northeast-1://my_app_modules//my-app/modules/main-module"
	sourceURL, err := toSourceUrl(source, ".")
	require.NoError(t, err)
	require.Equal(t, "git::codecommit::ap-northeast-1", sourceURL.Scheme)

	actualRootRepo, actualModulePath, err := splitSourceUrl(sourceURL, terragruntOptions.Logger)
	require.NoError(t, err)

	require.Equal(t, "git::codecommit::ap-northeast-1://my_app_modules", actualRootRepo.String())
	require.Equal(t, "my-app/modules/main-module", actualModulePath)
}
