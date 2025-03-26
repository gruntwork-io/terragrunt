package models_test

import (
	"fmt"
	"net/url"
	"testing"

	"github.com/gruntwork-io/terragrunt/tf/cache/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolveRelativeReferences(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		baseURL          string
		body             models.ResponseBody
		expectedResolved models.ResponseBody
	}{
		{
			"https://releases.hashicorp.com/terraform-provider-local/2.5.1",
			models.ResponseBody{
				DownloadURL:            "terraform-provider-local_2.5.1_darwin_amd64.zip",
				SHA256SumsURL:          "terraform-provider-local_2.5.1_SHA256SUMS",
				SHA256SumsSignatureURL: "terraform-provider-local_2.5.1_SHA256SUMS.72D7468F.sig",
			},
			models.ResponseBody{
				DownloadURL:            "https://releases.hashicorp.com/terraform-provider-local/2.5.1/terraform-provider-local_2.5.1_darwin_amd64.zip",
				SHA256SumsURL:          "https://releases.hashicorp.com/terraform-provider-local/2.5.1/terraform-provider-local_2.5.1_SHA256SUMS",
				SHA256SumsSignatureURL: "https://releases.hashicorp.com/terraform-provider-local/2.5.1/terraform-provider-local_2.5.1_SHA256SUMS.72D7468F.sig",
			},
		},
		{
			"https://somehost.com",
			models.ResponseBody{
				DownloadURL:            "https://releases.hashicorp.com/terraform-provider-local/2.5.1/terraform-provider-local_2.5.1_darwin_amd64.zip",
				SHA256SumsURL:          "https://releases.hashicorp.com/terraform-provider-local/2.5.1/terraform-provider-local_2.5.1_SHA256SUMS",
				SHA256SumsSignatureURL: "https://releases.hashicorp.com/terraform-provider-local/2.5.1/terraform-provider-local_2.5.1_SHA256SUMS.72D7468F.sig",
			},
			models.ResponseBody{
				DownloadURL:            "https://releases.hashicorp.com/terraform-provider-local/2.5.1/terraform-provider-local_2.5.1_darwin_amd64.zip",
				SHA256SumsURL:          "https://releases.hashicorp.com/terraform-provider-local/2.5.1/terraform-provider-local_2.5.1_SHA256SUMS",
				SHA256SumsSignatureURL: "https://releases.hashicorp.com/terraform-provider-local/2.5.1/terraform-provider-local_2.5.1_SHA256SUMS.72D7468F.sig",
			},
		},
	}

	for i, tc := range testCases {
		t.Run(fmt.Sprintf("testCase-%d", i), func(t *testing.T) {
			t.Parallel()

			baseURL, err := url.Parse(tc.baseURL)
			require.NoError(t, err)

			actualResolved := tc.body.ResolveRelativeReferences(baseURL)
			assert.Equal(t, tc.expectedResolved, *actualResolved)
		})
	}
}
