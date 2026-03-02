package models_test

import (
	"fmt"
	"net/url"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/tf/cache/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFilterValid(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name            string
		input           models.Versions
		expectedValid   []string
		expectedInvalid []string
	}{
		{
			name: "all valid versions",
			input: models.Versions{
				{Version: "1.0.0"},
				{Version: "2.5.2"},
				{Version: "0.1.0-beta1"},
			},
			expectedValid:   []string{"1.0.0", "2.5.2", "0.1.0-beta1"},
			expectedInvalid: nil,
		},
		{
			name: "v-prefixed versions are filtered",
			input: models.Versions{
				{Version: "1.0.0"},
				{Version: "v2.5.3"},
				{Version: "v1.0.0"},
			},
			expectedValid:   []string{"1.0.0"},
			expectedInvalid: []string{"v2.5.3", "v1.0.0"},
		},
		{
			name: "empty strings are filtered",
			input: models.Versions{
				{Version: "1.0.0"},
				{Version: ""},
				{Version: "2.0.0"},
			},
			expectedValid:   []string{"1.0.0", "2.0.0"},
			expectedInvalid: []string{""},
		},
		{
			name: "garbage strings are filtered",
			input: models.Versions{
				{Version: "1.0.0"},
				{Version: "not-a-version"},
				{Version: "latest"},
			},
			expectedValid:   []string{"1.0.0"},
			expectedInvalid: []string{"not-a-version", "latest"},
		},
		{
			name: "mixed valid and invalid",
			input: models.Versions{
				{Version: "1.0.0"},
				{Version: "v2.5.3-alpha1"},
				{Version: ""},
				{Version: "3.1.4"},
				{Version: "not-a-version"},
				{Version: "0.1.0-beta1"},
			},
			expectedValid:   []string{"1.0.0", "3.1.4", "0.1.0-beta1"},
			expectedInvalid: []string{"v2.5.3-alpha1", "", "not-a-version"},
		},
		{
			name:            "all invalid",
			input:           models.Versions{{Version: "v1.0.0"}, {Version: ""}, {Version: "bad"}},
			expectedValid:   nil,
			expectedInvalid: []string{"v1.0.0", "", "bad"},
		},
		{
			name:            "empty input",
			input:           models.Versions{},
			expectedValid:   nil,
			expectedInvalid: nil,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			valid, invalid := tc.input.FilterValid()

			var validStrs []string
			for _, v := range valid {
				validStrs = append(validStrs, v.Version)
			}

			assert.Equal(t, tc.expectedValid, validStrs)
			assert.Equal(t, tc.expectedInvalid, invalid)
		})
	}
}

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
		{
			"https://registry.company.com/v1/providers/ns/name/1.0/download/linux/amd64",
			models.ResponseBody{
				DownloadURL:            "/v1/providers/ns/name/1.0/download/linux/amd64/terraform-provider.zip",
				SHA256SumsURL:          "/v1/providers/ns/name/1.0/download/linux/amd64/terraform-provider_SHA256SUMS",
				SHA256SumsSignatureURL: "/v1/providers/ns/name/1.0/download/linux/amd64/terraform-provider_SHA256SUMS.sig",
			},
			models.ResponseBody{
				DownloadURL:            "https://registry.company.com/v1/providers/ns/name/1.0/download/linux/amd64/terraform-provider.zip",
				SHA256SumsURL:          "https://registry.company.com/v1/providers/ns/name/1.0/download/linux/amd64/terraform-provider_SHA256SUMS",
				SHA256SumsSignatureURL: "https://registry.company.com/v1/providers/ns/name/1.0/download/linux/amd64/terraform-provider_SHA256SUMS.sig",
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
