package tf_test

import (
	"fmt"
	"net/url"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/gruntwork-io/terragrunt/tf"
)

func TestSplitSourceUrl(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name               string
		sourceURL          string
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

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			sourceURL, err := url.Parse(tc.sourceURL)
			require.NoError(t, err)

			l := logger.CreateLogger()

			actualRootRepo, actualModulePath, err := tf.SplitSourceURL(l, sourceURL)
			require.NoError(t, err)

			assert.Equal(t, tc.expectedSo, actualRootRepo.String())
			assert.Equal(t, tc.expectedModulePath, actualModulePath)
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
		{"https://s3-eu-west-1.amazonaws.com/modules/vpc.zip", "https://s3-eu-west-1.amazonaws.com/modules/vpc.zip"},
		{"https://www.googleapis.com/storage/v1/modules/foomodule.zip", "gcs::https://www.googleapis.com/storage/v1/modules/foomodule.zip"},
		{"https://www.googleapis.com/storage/v1/modules/foomodule.zip", "gcs::https://www.googleapis.com/storage/v1/modules/foomodule.zip"},
		{"git::https://name@dev.azure.com/name/project-name/_git/repo-name", "git::https://name@dev.azure.com/name/project-name/_git/repo-name"},
		{"https://repository.rnd.net/artifactory/generic-production-iac/tf-auto-azr-iam.2.6.0.zip", "https://repository.rnd.net/artifactory/generic-production-iac/tf-auto-azr-iam.2.6.0.zip"},
	}

	for i, tc := range testCases {
		t.Run(fmt.Sprintf("testCase-%d", i), func(t *testing.T) {
			t.Parallel()

			actualSourceURL, err := tf.ToSourceURL(tc.sourceURL, os.TempDir())
			require.NoError(t, err)
			assert.Equal(t, tc.expectedSourceURL, actualSourceURL.String())
		})
	}
}

func TestRegressionSupportForGitRemoteCodecommit(t *testing.T) {
	t.Parallel()

	source := "git::codecommit::ap-northeast-1://my_app_modules//my-app/modules/main-module"
	sourceURL, err := tf.ToSourceURL(source, ".")
	require.NoError(t, err)
	require.Equal(t, "git::codecommit::ap-northeast-1", sourceURL.Scheme)

	l := logger.CreateLogger()

	actualRootRepo, actualModulePath, err := tf.SplitSourceURL(l, sourceURL)
	require.NoError(t, err)

	require.Equal(t, "git::codecommit::ap-northeast-1://my_app_modules", actualRootRepo.String())
	require.Equal(t, "my-app/modules/main-module", actualModulePath)
}
