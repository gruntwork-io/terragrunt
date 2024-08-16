package models_test

import (
	"fmt"
	"net/url"
	"testing"

	"github.com/gruntwork-io/terragrunt/terraform/cache/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolveRelativeReferences(t *testing.T) {
	t.Parallel()

	tc := []struct {
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

	for i, tt := range tc {
		tt := tt

		t.Run(fmt.Sprintf("testCase-%d", i), func(t *testing.T) {
			t.Parallel()

			baseURL, err := url.Parse(tt.baseURL)
			require.NoError(t, err)

			actualResolved := tt.body.ResolveRelativeReferences(baseURL)
			assert.Equal(t, tt.expectedResolved, *actualResolved)
		})
	}
}
