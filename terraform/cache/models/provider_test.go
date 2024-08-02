package models

import (
	"fmt"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolveRelativeReferences(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		baseURL          string
		body             ResponseBody
		expectedResolved ResponseBody
	}{
		{
			"https://releases.hashicorp.com/terraform-provider-local/2.5.1",
			ResponseBody{
				DownloadURL:            "terraform-provider-local_2.5.1_darwin_amd64.zip",
				SHA256SumsURL:          "terraform-provider-local_2.5.1_SHA256SUMS",
				SHA256SumsSignatureURL: "terraform-provider-local_2.5.1_SHA256SUMS.72D7468F.sig",
			},
			ResponseBody{
				DownloadURL:            "https://releases.hashicorp.com/terraform-provider-local/2.5.1/terraform-provider-local_2.5.1_darwin_amd64.zip",
				SHA256SumsURL:          "https://releases.hashicorp.com/terraform-provider-local/2.5.1/terraform-provider-local_2.5.1_SHA256SUMS",
				SHA256SumsSignatureURL: "https://releases.hashicorp.com/terraform-provider-local/2.5.1/terraform-provider-local_2.5.1_SHA256SUMS.72D7468F.sig",
			},
		},
		{
			"https://somehost.com",
			ResponseBody{
				DownloadURL:            "https://releases.hashicorp.com/terraform-provider-local/2.5.1/terraform-provider-local_2.5.1_darwin_amd64.zip",
				SHA256SumsURL:          "https://releases.hashicorp.com/terraform-provider-local/2.5.1/terraform-provider-local_2.5.1_SHA256SUMS",
				SHA256SumsSignatureURL: "https://releases.hashicorp.com/terraform-provider-local/2.5.1/terraform-provider-local_2.5.1_SHA256SUMS.72D7468F.sig",
			},
			ResponseBody{
				DownloadURL:            "https://releases.hashicorp.com/terraform-provider-local/2.5.1/terraform-provider-local_2.5.1_darwin_amd64.zip",
				SHA256SumsURL:          "https://releases.hashicorp.com/terraform-provider-local/2.5.1/terraform-provider-local_2.5.1_SHA256SUMS",
				SHA256SumsSignatureURL: "https://releases.hashicorp.com/terraform-provider-local/2.5.1/terraform-provider-local_2.5.1_SHA256SUMS.72D7468F.sig",
			},
		},
	}

	for i, testCase := range testCases {
		testCase := testCase

		t.Run(fmt.Sprintf("testCase-%d", i), func(t *testing.T) {
			t.Parallel()

			baseURL, err := url.Parse(testCase.baseURL)
			require.NoError(t, err)

			actualResolved := testCase.body.ResolveRelativeReferences(baseURL)
			assert.Equal(t, testCase.expectedResolved, *actualResolved)
		})
	}
}
