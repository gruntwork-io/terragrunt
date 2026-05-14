package models_test

import (
	"encoding/json"
	"fmt"
	"net/url"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/tf/cache/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResponseBodyPackagesRoundTrip(t *testing.T) {
	t.Parallel()

	// Sample response shape extracted from opentofu/opentofu RFC
	// 20251027-provider-registry-hashes.md, including the `packages` field that
	// carries per-platform hashes.
	raw := []byte(`{
		"protocols": ["5.0"],
		"os": "linux",
		"arch": "amd64",
		"filename": "terraform-provider-aws_5.31.0_linux_amd64.zip",
		"download_url": "https://example.com/terraform-provider-aws_5.31.0_linux_amd64.zip",
		"shasums_url": "https://example.com/SHA256SUMS",
		"shasum": "5f9c7aa76b7c34d722fc91111111111111111111111111111111111111111111",
		"signing_keys": {"gpg_public_keys": []},
		"packages": {
			"linux_amd64": {
				"hashes": ["zh:abc", "h1:def"],
				"package_size": 12345
			},
			"darwin_arm64": {
				"hashes": ["zh:ghi", "h1:jkl"],
				"package_size": 67890
			}
		}
	}`)

	var body models.ResponseBody
	require.NoError(t, json.Unmarshal(raw, &body))

	require.Len(t, body.Packages, 2)
	require.Contains(t, body.Packages, "linux_amd64")
	assert.Equal(t, []string{"zh:abc", "h1:def"}, body.Packages["linux_amd64"].Hashes)
	assert.Equal(t, int64(12345), body.Packages["linux_amd64"].PackageSize)
	assert.Equal(t, []string{"zh:ghi", "h1:jkl"}, body.Packages["darwin_arm64"].Hashes)
}

func TestResponseBodyPackagesAbsent(t *testing.T) {
	t.Parallel()

	raw := []byte(`{"os":"linux","arch":"amd64","filename":"x.zip","download_url":"https://example.com/x.zip","signing_keys":{"gpg_public_keys":[]}}`)

	var body models.ResponseBody
	require.NoError(t, json.Unmarshal(raw, &body))

	assert.Nil(t, body.Packages)
}

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
			expectedInvalid: []string{},
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
			expectedValid:   []string{},
			expectedInvalid: []string{"v1.0.0", "", "bad"},
		},
		{
			name:            "empty input",
			input:           models.Versions{},
			expectedValid:   []string{},
			expectedInvalid: []string{},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			valid, invalid := tc.input.FilterValid()

			validStrs := make([]string, 0, len(valid))
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
			baseURL: "https://releases.hashicorp.com/terraform-provider-local/2.5.1",
			body: models.ResponseBody{
				DownloadURL:            "terraform-provider-local_2.5.1_darwin_amd64.zip",
				SHA256SumsURL:          "terraform-provider-local_2.5.1_SHA256SUMS",
				SHA256SumsSignatureURL: "terraform-provider-local_2.5.1_SHA256SUMS.72D7468F.sig",
			},
			expectedResolved: models.ResponseBody{
				DownloadURL:            "https://releases.hashicorp.com/terraform-provider-local/2.5.1/terraform-provider-local_2.5.1_darwin_amd64.zip",
				SHA256SumsURL:          "https://releases.hashicorp.com/terraform-provider-local/2.5.1/terraform-provider-local_2.5.1_SHA256SUMS",
				SHA256SumsSignatureURL: "https://releases.hashicorp.com/terraform-provider-local/2.5.1/terraform-provider-local_2.5.1_SHA256SUMS.72D7468F.sig",
			},
		},
		{
			baseURL: "https://somehost.com",
			body: models.ResponseBody{
				DownloadURL:            "https://releases.hashicorp.com/terraform-provider-local/2.5.1/terraform-provider-local_2.5.1_darwin_amd64.zip",
				SHA256SumsURL:          "https://releases.hashicorp.com/terraform-provider-local/2.5.1/terraform-provider-local_2.5.1_SHA256SUMS",
				SHA256SumsSignatureURL: "https://releases.hashicorp.com/terraform-provider-local/2.5.1/terraform-provider-local_2.5.1_SHA256SUMS.72D7468F.sig",
			},
			expectedResolved: models.ResponseBody{
				DownloadURL:            "https://releases.hashicorp.com/terraform-provider-local/2.5.1/terraform-provider-local_2.5.1_darwin_amd64.zip",
				SHA256SumsURL:          "https://releases.hashicorp.com/terraform-provider-local/2.5.1/terraform-provider-local_2.5.1_SHA256SUMS",
				SHA256SumsSignatureURL: "https://releases.hashicorp.com/terraform-provider-local/2.5.1/terraform-provider-local_2.5.1_SHA256SUMS.72D7468F.sig",
			},
		},
		{
			baseURL: "https://registry.company.com/v1/providers/ns/name/1.0/download/linux/amd64",
			body: models.ResponseBody{
				DownloadURL:            "/v1/providers/ns/name/1.0/download/linux/amd64/terraform-provider.zip",
				SHA256SumsURL:          "/v1/providers/ns/name/1.0/download/linux/amd64/terraform-provider_SHA256SUMS",
				SHA256SumsSignatureURL: "/v1/providers/ns/name/1.0/download/linux/amd64/terraform-provider_SHA256SUMS.sig",
			},
			expectedResolved: models.ResponseBody{
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
