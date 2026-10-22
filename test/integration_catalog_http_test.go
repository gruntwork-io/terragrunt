//go:build http

package test_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gruntwork-io/terragrunt/test/helpers"
)

const (
	fakeModulesTreeURL = "https://github.com/gruntwork-io/terraform-fake-modules/tree/"

	// fakeModulesV005Commit is the commit the v0.0.5 tag points at. A clone
	// made without the CAS checks the tag out detached, so its links name the
	// commit.
	fakeModulesV005Commit = "321a362579ae52f388d1585a4409e03f872d420b"
)

// TestHTTPCatalogJSONLURLsResolve pins that every `url` the catalog reports
// for a repository on github.com answers 200, with and without a //subdir in
// the catalog URL, and with the CAS disabled.
func TestHTTPCatalogJSONLURLsResolve(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name       string
		flags      string
		catalogURL string
		auroraDir  string
		auroraURL  string
	}{
		{
			name:       "repository root",
			catalogURL: "github.com/gruntwork-io/terraform-fake-modules?ref=v0.0.5",
			auroraDir:  "modules/aws/aurora",
			auroraURL:  fakeModulesTreeURL + "v0.0.5/modules/aws/aurora",
		},
		{
			name:       "subdir",
			catalogURL: "github.com/gruntwork-io/terraform-fake-modules//modules?ref=v0.0.5",
			auroraDir:  "aws/aurora",
			auroraURL:  fakeModulesTreeURL + "v0.0.5/modules/aws/aurora",
		},
		{
			name:       "repository root without CAS",
			flags:      "--no-cas ",
			catalogURL: "github.com/gruntwork-io/terraform-fake-modules?ref=v0.0.5",
			auroraDir:  "modules/aws/aurora",
			auroraURL:  fakeModulesTreeURL + fakeModulesV005Commit + "/modules/aws/aurora",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			stdout, _, err := helpers.RunTerragruntCommandWithOutput(t,
				"terragrunt catalog --format jsonl "+tc.flags+"--working-dir "+
					helpers.TmpDirWOSymlinks(t)+" "+tc.catalogURL,
			)
			require.NoError(t, err)

			byDir := parseCatalogJSONL(t, stdout)
			require.Contains(t, byDir, tc.auroraDir)

			assert.Equal(t, tc.auroraURL, byDir[tc.auroraDir].URL)

			for dir, entry := range byDir {
				req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, entry.URL, nil)
				require.NoError(t, err)

				resp, err := http.DefaultClient.Do(req)
				require.NoError(t, err)
				require.NoError(t, resp.Body.Close())

				assert.Equal(
					t,
					http.StatusOK,
					resp.StatusCode,
					"url %q of component %q",
					entry.URL,
					dir,
				)
			}
		})
	}
}
