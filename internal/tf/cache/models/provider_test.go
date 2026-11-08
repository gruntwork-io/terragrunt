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

	raw := []byte(
		`{"os":"linux","arch":"amd64","filename":"x.zip","download_url":"https://example.com/x.zip","signing_keys":{"gpg_public_keys":[]}}`,
	)

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

func TestFilenameFromURL(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "plain filename",
			input:    "terraform-provider-aws_5.0.0_linux_amd64.zip",
			expected: "terraform-provider-aws_5.0.0_linux_amd64.zip",
		},
		{
			name:     "URL without query string",
			input:    "https://releases.hashicorp.com/terraform-provider-aws_5.0.0_linux_amd64.zip",
			expected: "terraform-provider-aws_5.0.0_linux_amd64.zip",
		},
		{
			name:     "signed S3 URL with query string",
			input:    "https://s3.amazonaws.com/bucket/terraform-provider-aws_5.0.0_linux_amd64.zip?X-Amz-Algorithm=AWS4-HMAC-SHA256&X-Amz-Credential=fake-access-key-id%2F20130524%2Fus-east-1%2Fs3%2Faws4_request&X-Amz-Date=20130524T000000Z&X-Amz-Expires=86400&X-Amz-SignedHeaders=host&X-Amz-Signature=fake-signature",
			expected: "terraform-provider-aws_5.0.0_linux_amd64.zip",
		},
		{
			name:     "URL with fragment",
			input:    "https://example.com/provider.zip#section",
			expected: "provider.zip",
		},
		{
			name:     "URL with query and fragment",
			input:    "https://example.com/path/provider.zip?token=abc#section",
			expected: "provider.zip",
		},
		{
			name:     "relative URL with query string",
			input:    "terraform-provider-aws_5.0.0_linux_amd64.zip?v=1",
			expected: "terraform-provider-aws_5.0.0_linux_amd64.zip",
		},
		{
			name:     "URL with deeply nested path",
			input:    "https://registry.example.com/v1/providers/hashicorp/aws/5.0.0/download/linux/amd64/terraform-provider-aws.zip?signed=true",
			expected: "terraform-provider-aws.zip",
		},
		{
			name:     "Windows local mirror path",
			input:    `C:\mirror\registry.example.com\hashicorp\aws\terraform-provider-aws_5.0.0_windows_amd64.zip`,
			expected: "terraform-provider-aws_5.0.0_windows_amd64.zip",
		},
		{
			name:     "Windows UNC mirror path",
			input:    `\\server\share\mirror\terraform-provider-aws_5.0.0_windows_amd64.zip`,
			expected: "terraform-provider-aws_5.0.0_windows_amd64.zip",
		},
		{
			name:     "empty string",
			input:    "",
			expected: ".",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			result := models.FilenameFromURL(tc.input)
			assert.Equal(t, tc.expected, result)
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
		{
			baseURL: "https://registry.company.com/v1/providers/ns/name/1.0/download/linux/amd64",
			body: models.ResponseBody{
				DownloadURL:            "terraform-provider.zip?X-Amz-Signature=abc123&X-Amz-Expires=86400",
				SHA256SumsURL:          "terraform-provider_SHA256SUMS?X-Amz-Signature=def456",
				SHA256SumsSignatureURL: "terraform-provider_SHA256SUMS.sig#anchor",
			},
			expectedResolved: models.ResponseBody{
				DownloadURL:            "https://registry.company.com/v1/providers/ns/name/1.0/download/linux/amd64/terraform-provider.zip?X-Amz-Signature=abc123&X-Amz-Expires=86400",
				SHA256SumsURL:          "https://registry.company.com/v1/providers/ns/name/1.0/download/linux/amd64/terraform-provider_SHA256SUMS?X-Amz-Signature=def456",
				SHA256SumsSignatureURL: "https://registry.company.com/v1/providers/ns/name/1.0/download/linux/amd64/terraform-provider_SHA256SUMS.sig#anchor",
			},
		},
		{
			baseURL: "https://registry.company.com/v1/providers/ns/name/1.0/download/linux/amd64",
			body: models.ResponseBody{
				DownloadURL:   "/downloads/terraform-provider.zip?X-Amz-Signature=abc123",
				SHA256SumsURL: "/downloads/terraform-provider_SHA256SUMS",
			},
			expectedResolved: models.ResponseBody{
				DownloadURL:   "https://registry.company.com/downloads/terraform-provider.zip?X-Amz-Signature=abc123",
				SHA256SumsURL: "https://registry.company.com/downloads/terraform-provider_SHA256SUMS",
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

func TestResolveRelativeReference(t *testing.T) {
	t.Parallel()

	const (
		registry = "https://registry.example.com"
		platform = registry + "/v1/providers/ns/name/1.0/download/linux/amd64"
	)

	testCases := []struct {
		name string
		base string
		link string
		want string
	}{
		{name: "file name", base: platform, link: "example.zip", want: platform + "/example.zip"},
		{
			name: "URL in the query",
			base: platform,
			link: "example.zip?redirect=https://mirror.example.com/example.zip",
			want: platform + "/example.zip?redirect=https://mirror.example.com/example.zip",
		},
		{
			name: "escaped slash",
			base: platform,
			link: "group%2Fproject/example.zip",
			want: platform + "/group%2Fproject/example.zip",
		},
		{name: "repeated slashes", base: platform, link: "files//example.zip", want: platform + "/files//example.zip"},
		{name: "trailing slash", base: platform, link: "files/", want: platform + "/files/"},
		{
			name: "parent directory",
			base: platform,
			link: "../SHA256SUMS",
			want: registry + "/v1/providers/ns/name/1.0/download/linux/SHA256SUMS",
		},
		{name: "absolute path", base: platform, link: "/sums/SHA256SUMS", want: registry + "/sums/SHA256SUMS"},
		{name: "query only", base: platform, link: "?sig=1", want: platform + "?sig=1"},
		{
			name: "network path",
			base: platform,
			link: "//mirror.example.com/example.zip",
			want: "https://mirror.example.com/example.zip",
		},
		{
			name: "absolute URL",
			base: platform,
			link: "https://mirror.example.com/a//b/../example.zip",
			want: "https://mirror.example.com/a//b/../example.zip",
		},
		{
			name: "base ending in a slash",
			base: "https://mirror.example.com/api/",
			link: "example.zip",
			want: "https://mirror.example.com/api/example.zip",
		},
		{
			name: "base with an escaped slash",
			base: "https://mirror.example.com/group%2Fmirror",
			link: "example.zip",
			want: "https://mirror.example.com/group%2Fmirror/example.zip",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			base, err := url.Parse(tc.base)
			require.NoError(t, err)

			assert.Equal(t, tc.want, models.ResolveRelativeReference(base, tc.link))
		})
	}
}

func TestParseProvider(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name  string
		input string
		want  models.Provider
	}{
		{
			name:  "registry namespace and name",
			input: "registry.terraform.io/hashicorp/aws",
			want:  models.Provider{RegistryName: "registry.terraform.io", Namespace: "hashicorp", Name: "aws"},
		},
		{
			name:  "namespace and name",
			input: "hashicorp/aws",
			want:  models.Provider{Namespace: "hashicorp", Name: "aws"},
		},
		{
			name:  "registry only",
			input: "registry.terraform.io",
			want:  models.Provider{RegistryName: "registry.terraform.io"},
		},
		{
			name:  "wildcard parts become empty",
			input: "registry.terraform.io/*/*",
			want:  models.Provider{RegistryName: "registry.terraform.io"},
		},
		{
			name:  "extra parts are ignored",
			input: "registry.terraform.io/hashicorp/aws/extra",
			want:  models.Provider{RegistryName: "registry.terraform.io", Namespace: "hashicorp", Name: "aws"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := models.ParseProvider(tc.input)
			require.NotNil(t, got)
			assert.Equal(t, tc.want, *got, "parsed provider should match the address parts")
		})
	}
}

func TestParseProviders(t *testing.T) {
	t.Parallel()

	got := models.ParseProviders("registry.terraform.io/hashicorp/aws", "hashicorp/null")

	require.Len(t, got, 2)
	assert.Equal(t, "registry.terraform.io/hashicorp/aws", got[0].Address())
	assert.Equal(t, "hashicorp/null", got[1].Address())
	assert.Empty(t, models.ParseProviders(), "no input should give no providers")
}

func TestProvidersFind(t *testing.T) {
	t.Parallel()

	providers := models.ParseProviders("registry.terraform.io/hashicorp/aws", "registry.terraform.io/hashicorp/null")

	found := providers.Find(models.ParseProvider("registry.terraform.io/hashicorp/null"))
	require.NotNil(t, found, "matching provider should be found")
	assert.Equal(t, "null", found.Name)

	assert.Nil(
		t,
		providers.Find(models.ParseProvider("registry.terraform.io/hashicorp/random")),
		"unknown provider should not be found",
	)
}

func TestProviderMatch(t *testing.T) {
	t.Parallel()

	full := &models.Provider{
		ResponseBody: &models.ResponseBody{DownloadURL: "https://example.com/aws.zip"},
		RegistryName: "registry.terraform.io",
		Namespace:    "hashicorp",
		Name:         "aws",
		OS:           "linux",
		Arch:         "amd64",
	}

	testCases := []struct {
		provider *models.Provider
		target   *models.Provider
		name     string
		want     bool
	}{
		{
			name:     "identical providers",
			provider: full,
			target:   full,
			want:     true,
		},
		{
			name:     "empty provider matches anything",
			provider: &models.Provider{},
			target:   full,
			want:     true,
		},
		{
			name:     "empty target matches anything",
			provider: full,
			target:   &models.Provider{ResponseBody: &models.ResponseBody{}},
			want:     true,
		},
		{
			name:     "different registry",
			provider: full,
			target:   &models.Provider{ResponseBody: &models.ResponseBody{}, RegistryName: "registry.opentofu.org"},
		},
		{
			name:     "different namespace",
			provider: full,
			target:   &models.Provider{ResponseBody: &models.ResponseBody{}, Namespace: "opentofu"},
		},
		{
			name:     "different name",
			provider: full,
			target:   &models.Provider{ResponseBody: &models.ResponseBody{}, Name: "null"},
		},
		{
			name:     "different os",
			provider: full,
			target:   &models.Provider{ResponseBody: &models.ResponseBody{}, OS: "darwin"},
		},
		{
			name:     "different arch",
			provider: full,
			target:   &models.Provider{ResponseBody: &models.ResponseBody{}, Arch: "arm64"},
		},
		{
			name:     "different download url",
			provider: full,
			target:   &models.Provider{ResponseBody: &models.ResponseBody{DownloadURL: "https://example.com/other.zip"}},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tc.want, tc.provider.Match(tc.target), "match should require every set property to be equal")
		})
	}
}

func TestProviderStrings(t *testing.T) {
	t.Parallel()

	provider := &models.Provider{
		RegistryName:        "registry.terraform.io",
		Namespace:           "hashicorp",
		Name:                "aws",
		OS:                  "linux",
		Arch:                "amd64",
		OriginalConstraints: ">= 5.0",
	}

	assert.Equal(
		t,
		"registry.terraform.io/hashicorp/aws",
		provider.String(),
		"unversioned provider should omit the version",
	)
	assert.Equal(t, "registry.terraform.io/hashicorp/aws", provider.Address())
	assert.Equal(t, "linux_amd64", provider.Platform())
	assert.Equal(t, ">= 5.0", provider.Constraints())

	provider.Version = "5.31.0"

	assert.Equal(
		t,
		"registry.terraform.io/hashicorp/aws v5.31.0",
		provider.String(),
		"versioned provider should show the version",
	)
}

func TestVersionAndPlatformString(t *testing.T) {
	t.Parallel()

	platform := models.Platform{OS: "linux", Arch: "amd64"}
	assert.Equal(t, "linux/amd64", platform.String())

	version := models.Version{
		Version:   "5.31.0",
		Protocols: []string{"5.0"},
		Platforms: models.Platforms{&platform},
	}
	assert.Equal(t, "5.31.0/[5.0]/[linux/amd64]", version.String())
}

func TestSigningKeyListKeys(t *testing.T) {
	t.Parallel()

	list := models.SigningKeyList{
		GPGPublicKeys: []*models.SigningKey{
			{ASCIIArmor: "armor-a", TrustSignature: "sig-a"},
			{ASCIIArmor: "armor-b"},
		},
	}

	assert.Equal(
		t,
		map[string]string{"armor-a": "sig-a", "armor-b": ""},
		list.Keys(),
		"keys should map armor to trust signature",
	)
	assert.Empty(t, models.SigningKeyList{}.Keys(), "no keys should give an empty map")
}

func TestResolveRelativeReferencesUnparsableLinkIsKept(t *testing.T) {
	t.Parallel()

	base, err := url.Parse("https://registry.example.com/v1")
	require.NoError(t, err)

	body := models.ResponseBody{DownloadURL: "%zz.zip"}

	assert.Equal(
		t,
		"%zz.zip",
		body.ResolveRelativeReferences(base).DownloadURL,
		"a link that does not parse should be returned unchanged",
	)
}
