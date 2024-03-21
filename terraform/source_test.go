package terraform

import (
	"net/url"
	"testing"

	"github.com/hashicorp/go-getter"
	urlhelper "github.com/hashicorp/go-getter/helper/url"

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
		{"parent-url-one-child-with-double-slash", "ssh://git@github.com/foo/modules.git//foo", "ssh://git@github.com/foo/modules.git?depth=1", "foo"},
		{"parent-url-multiple-children-with-double-slash", "ssh://git@github.com/foo/modules.git//foo/bar/baz/blah", "ssh://git@github.com/foo/modules.git?depth=1", "foo/bar/baz/blah"},
		{"separate-ref-with-slash", "ssh://git@github.com/foo/modules.git//foo?ref=feature/modules", "ssh://git@github.com/foo/modules.git?depth=1&ref=feature%2Fmodules", "foo"},
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

			actualRootRepo, actualModulePath, err := SplitSourceUrl(sourceUrl, terragruntOptions.Logger)
			require.NoError(t, err)

			assert.Equal(t, testCase.expectedRootRepo, actualRootRepo.String())
			assert.Equal(t, testCase.expectedModulePath, actualModulePath)
		})
	}
}

func TestIsGitUrl(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name          string
		sourceUrl     string
		forcedGetters []string
		expected      bool
	}{
		{"git scheme", "git://example.com/owner/repo", nil, true},
		{"git+ssh scheme", "git+ssh://example.com/owner/repo", nil, true},
		{"ssh scheme", "ssh://example.com/owner/repo", nil, true},
		{"git/ssh URL", "git@example.com:owner/repo", nil, true},
		{"https scheme, github", "https://github.com/owner/repo", nil, true},
		{"https scheme, gitlab", "https://gitlab.com/owner/repo", nil, true},
		{"https scheme, bitbucket", "https://bitbucket.org/owner/repo", nil, true},
		{"https scheme, path contains .git", "https://example.com/owner/repo.git", nil, true},
		{"https scheme unknown host / pattern", "https://example.com/owner/repo", nil, false},
		{"https scheme with git forcedGetter", "https://example.com/owner/repo", []string{"git"}, true},
		{"https scheme with other forcedGetter", "https://example.com/owner/repo", []string{"scp"}, false},
		{"realistic GitHub HTTPS URL", "https://github.com/owner/repo.git", nil, true},
		{"realistic GitHub SSH URL", "git@github.com:owner/repo.git", nil, true},
		{"realistic GitHub HTTPS URL with module", "https://github.com/owner/repo.git//modules/vpc", nil, true},
		{"realistic GitHub SSH URL with module", "git@github.com:owner/repo.git//modules/vpc", nil, true},
	}

	for _, testCase := range testCases {
		// Save a local copy in scope so all the tests don't run the final item in the loop
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			sourceUrlWithGetter, err := getter.Detect(testCase.sourceUrl, ".", getter.Detectors)
			require.NoError(t, err)

			sourceUrl, err := urlhelper.Parse(sourceUrlWithGetter)
			require.NoError(t, err)

			actual := isGitUrl(sourceUrl, testCase.forcedGetters)
			assert.Equal(t, testCase.expected, actual, "For source URL %s and forcedGetters %v", testCase.sourceUrl, testCase.forcedGetters)
		})
	}
}

func TestRegressionSupportForGitRemoteCodecommit(t *testing.T) {
	t.Parallel()

	terragruntOptions, err := options.NewTerragruntOptionsForTest("testing")
	require.NoError(t, err)

	source := "git::codecommit::ap-northeast-1://my_app_modules//my-app/modules/main-module"
	sourceURL, err := ToSourceUrl(source, ".")
	require.NoError(t, err)
	require.Equal(t, "git::codecommit::ap-northeast-1", sourceURL.Scheme)

	actualRootRepo, actualModulePath, err := SplitSourceUrl(sourceURL, terragruntOptions.Logger)
	require.NoError(t, err)

	require.Equal(t, "git::codecommit::ap-northeast-1://my_app_modules?depth=1", actualRootRepo.String())
	require.Equal(t, "my-app/modules/main-module", actualModulePath)
}
