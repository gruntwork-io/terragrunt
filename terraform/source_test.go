package terraform

import (
	"fmt"
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
		expectedSo         string
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

			actualRootRepo, actualModulePath, err := SplitSourceUrl(sourceUrl, terragruntOptions.Logger)
			require.NoError(t, err)

			assert.Equal(t, testCase.expectedSo, actualRootRepo.String())
			assert.Equal(t, testCase.expectedModulePath, actualModulePath)
		})
	}
}

func TestToSourceUrl(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		sourceURL         string
		expectedSourceURL string
	}{
		{"https://github.com/gruntwork-io/repo-name", "git::https://github.com/gruntwork-io/repo-name.git"},
		{"git::https://github.com/gruntwork-io/repo-name", "git::https://github.com/gruntwork-io/repo-name"},
		{"https://github.com/gruntwork-io/repo-name//modules/module-name", "git::https://github.com/gruntwork-io/repo-name.git//modules/module-name"},
		{"ssh://github.com/gruntwork-io/repo-name//modules/module-name", "ssh://github.com/gruntwork-io/repo-name//modules/module-name"},
		{"https://gitlab.com/catamphetamine/libphonenumber-js", "git::https://gitlab.com/catamphetamine/libphonenumber-js.git"},
		{"https://bitbucket.org/atlassian/aws-ecr-push-image", "git::https://bitbucket.org/atlassian/aws-ecr-push-image.git"},
		{"http://bitbucket.org/atlassian/aws-ecr-push-image", "git::https://bitbucket.org/atlassian/aws-ecr-push-image.git"},
		{"https://s3-eu-west-1.amazonaws.com/modules/vpc.zip", "s3::https://s3-eu-west-1.amazonaws.com/modules/vpc.zip"},
		{"https://www.googleapis.com/storage/v1/modules/foomodule.zip", "gcs::https://www.googleapis.com/storage/v1/modules/foomodule.zip"},
		{"https://www.googleapis.com/storage/v1/modules/foomodule.zip", "gcs::https://www.googleapis.com/storage/v1/modules/foomodule.zip"},
		{"git::https://name@dev.azure.com/name/project-name/_git/repo-name", "git::https://name@dev.azure.com/name/project-name/_git/repo-name"},
	}

	for i, testCase := range testCases {
		// Save a local copy in scope so all the tests don't run the final item in the loop
		testCase := testCase
		t.Run(fmt.Sprintf("testCase-%d", i), func(t *testing.T) {
			t.Parallel()

			actualSourceURL, err := ToSourceUrl(testCase.sourceURL, "")
			require.NoError(t, err)
			assert.Equal(t, testCase.expectedSourceURL, actualSourceURL.String())
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

	require.Equal(t, "git::codecommit::ap-northeast-1://my_app_modules", actualRootRepo.String())
	require.Equal(t, "my-app/modules/main-module", actualModulePath)
}
