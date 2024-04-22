package provider

import (
	"crypto/sha256"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPackageAuthenticationResult(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		result   *PackageAuthenticationResult
		expected string
	}{
		{
			nil,
			"unauthenticated",
		},
		{
			NewPackageAuthenticationResult(verifiedChecksum),
			"verified checksum",
		},
		{
			NewPackageAuthenticationResult(officialProvider),
			"signed by HashiCorp",
		},
		{
			NewPackageAuthenticationResult(partnerProvider),
			"signed by a HashiCorp partner",
		},
		{
			NewPackageAuthenticationResult(communityProvider),
			"self-signed",
		},
	}

	for i, testCase := range testCases {
		testCase := testCase

		t.Run(fmt.Sprintf("testCase-%d", i), func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, testCase.expected, testCase.result.String())
		})
	}
}

func TestArchiveChecksumAuthentication_success(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		path           string
		wantSHA256Sum  [sha256.Size]byte
		expectedResult *PackageAuthenticationResult
		expectedErr    error
	}{
		{
			"testdata/filesystem-mirror/registry.terraform.io/hashicorp/null/terraform-provider-null_2.1.0_linux_amd64.zip",
			[sha256.Size]byte{
				0x4f, 0xb3, 0x98, 0x49, 0xf2, 0xe1, 0x38, 0xeb,
				0x16, 0xa1, 0x8b, 0xa0, 0xc6, 0x82, 0x63, 0x5d,
				0x78, 0x1c, 0xb8, 0xc3, 0xb2, 0x59, 0x01, 0xdd,
				0x5a, 0x79, 0x2a, 0xde, 0x97, 0x11, 0xf5, 0x01,
			},
			NewPackageAuthenticationResult(verifiedChecksum),
			nil,
		},
		{
			"testdata/filesystem-mirror/registry.terraform.io/hashicorp/null/terraform-provider-null_invalid.zip",
			[sha256.Size]byte{
				0x4f, 0xb3, 0x98, 0x49, 0xf2, 0xe1, 0x38, 0xeb,
				0x16, 0xa1, 0x8b, 0xa0, 0xc6, 0x82, 0x63, 0x5d,
				0x78, 0x1c, 0xb8, 0xc3, 0xb2, 0x59, 0x01, 0xdd,
				0x5a, 0x79, 0x2a, 0xde, 0x97, 0x11, 0xf5, 0x01,
			},
			nil,
			fmt.Errorf("archive has incorrect checksum zh:8610a6d93c01e05a0d3920fe66c79b3c7c3b084f1f5c70715afd919fee1d978e (expected zh:4fb39849f2e138eb16a18ba0c682635d781cb8c3b25901dd5a792ade9711f501)"),
		},
		{
			"testdata/no-package-here.zip",
			[sha256.Size]byte{},
			nil,
			fmt.Errorf("stat testdata/no-package-here.zip: no such file or directory"),
		},
		{
			"testdata/filesystem-mirror/registry.terraform.io/hashicorp/null/terraform-provider-null_2.1.0_linux_amd64.zip",
			[sha256.Size]byte{},
			nil,
			fmt.Errorf("archive has incorrect checksum zh:4fb39849f2e138eb16a18ba0c682635d781cb8c3b25901dd5a792ade9711f501 (expected zh:0000000000000000000000000000000000000000000000000000000000000000)"),
		},
		{
			"testdata/filesystem-mirror/tfe.example.com/AwesomeCorp/happycloud/0.1.0-alpha.2/darwin_amd64",
			[sha256.Size]byte{},
			nil,
			fmt.Errorf("cannot check archive hash for non-archive location testdata/filesystem-mirror/tfe.example.com/AwesomeCorp/happycloud/0.1.0-alpha.2/darwin_amd64"),
		},
	}

	for i, testCase := range testCases {
		testCase := testCase

		t.Run(fmt.Sprintf("testCase-%d", i), func(t *testing.T) {
			t.Parallel()

			auth := NewArchiveChecksumAuthentication(testCase.wantSHA256Sum)
			actualResult, err := auth.Authenticate(testCase.path)
			if testCase.expectedErr != nil {
				require.EqualError(t, err, testCase.expectedErr.Error())
			} else {
				require.NoError(t, err)
			}

			assert.Equal(t, testCase.expectedResult, actualResult)
		})

	}
}

func TestNewMatchingChecksumAuthentication(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		path          string
		filename      string
		document      []byte
		wantSHA256Sum [sha256.Size]byte
		expectedErr   error
	}{
		{
			"testdata/my-package.zip",
			"my-package.zip",
			[]byte(fmt.Sprintf("%x README.txt\n%x my-package.zip\n", [sha256.Size]byte{0xc0, 0xff, 0xee}, [sha256.Size]byte{0xde, 0xca, 0xde})),
			[sha256.Size]byte{0xde, 0xca, 0xde},
			nil,
		},

		{
			"testdata/my-package.zip",
			"my-package.zip",
			[]byte(
				fmt.Sprintf(
					"%x README.txt",
					[sha256.Size]byte{0xbe, 0xef},
				),
			),
			[sha256.Size]byte{0xde, 0xca, 0xde},
			fmt.Errorf(`checksum list has no SHA-256 hash for "my-package.zip"`),
		},
		{
			"testdata/my-package.zip",
			"my-package.zip",
			[]byte(
				fmt.Sprintf(
					"%s README.txt\n%s my-package.zip",
					"horses",
					"chickens",
				),
			),
			[sha256.Size]byte{0xde, 0xca, 0xde},
			fmt.Errorf(`checksum list has invalid SHA256 hash "chickens": encoding/hex: invalid byte: U+0068 'h'`),
		},
		{
			"testdata/my-package.zip",
			"my-package.zip",
			[]byte(
				fmt.Sprintf(
					"%x README.txt\n%x my-package.zip",
					[sha256.Size]byte{0xbe, 0xef},
					[sha256.Size]byte{0xc0, 0xff, 0xee},
				),
			),
			[sha256.Size]byte{0xde, 0xca, 0xde},
			fmt.Errorf("checksum list has unexpected SHA-256 hash c0ffee0000000000000000000000000000000000000000000000000000000000 (expected decade0000000000000000000000000000000000000000000000000000000000)"),
		},
	}

	for i, testCase := range testCases {
		testCase := testCase

		t.Run(fmt.Sprintf("testCase-%d", i), func(t *testing.T) {
			t.Parallel()

			auth := NewMatchingChecksumAuthentication(testCase.document, testCase.filename, testCase.wantSHA256Sum)
			_, err := auth.Authenticate(testCase.path)

			if testCase.expectedErr != nil {
				require.EqualError(t, err, testCase.expectedErr.Error())
			} else {
				require.NoError(t, err)
			}

		})
	}
}

func TestSignatureAuthentication(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		shasums        string
		expectedHashes []Hash
	}{
		{
			`7d7e888fdd28abfe00894f9055209b9eec785153641de98e6852aa071008d4ee  terraform_0.14.0-alpha20200923_darwin_amd64.zip
f8b6cf9ade087c17826d49d89cef21261cdc22bd27065bbc5b27d7dbf7fbbf6c  terraform_0.14.0-alpha20200923_freebsd_386.zip
a5ba9945606bb7bfb821ba303957eeb40dd9ee4e706ba8da1eaf7cbeb0356e63  terraform_0.14.0-alpha20200923_freebsd_amd64.zip
df3a5a8d6ffff7bacf19c92d10d0d500f98169ea17b3764b01a789f563d1aad7  terraform_0.14.0-alpha20200923_freebsd_arm.zip
086119a26576d06b8281a97e8644380da89ce16197cd955f74ea5ee664e9358b  terraform_0.14.0-alpha20200923_linux_386.zip
1e5f7a5f3ade7b8b1d1d59c5cea2e1a2f8d2f8c3f41962dbbe8647e222be8239  terraform_0.14.0-alpha20200923_linux_amd64.zip
0e9fd0f3e2254b526a0e81e0cfdfc82583b0cd343778c53ead21aa7d52f776d7  terraform_0.14.0-alpha20200923_linux_arm.zip
66a947e7de1c74caf9f584c3ed4e91d2cb1af6fe5ce8abaf1cf8f7ff626a09d1  terraform_0.14.0-alpha20200923_openbsd_386.zip
def1b73849bec0dc57a04405847921bf9206c75b52ae9de195476facb26bd85e  terraform_0.14.0-alpha20200923_openbsd_amd64.zip
48f1826ec31d6f104e46cc2022b41f30cd1019ef48eaec9697654ef9ec37a879  terraform_0.14.0-alpha20200923_solaris_amd64.zip
17e0b496022bc4e4137be15e96d2b051c8acd6e14cb48d9b13b262330464f6cc  terraform_0.14.0-alpha20200923_windows_386.zip
2696c86228f491bc5425561c45904c9ce39b1c676b1e17734cb2ee6b578c4bcd  terraform_0.14.0-alpha20200923_windows_amd64.zip`,
			[]Hash{
				"zh:7d7e888fdd28abfe00894f9055209b9eec785153641de98e6852aa071008d4ee",
				"zh:f8b6cf9ade087c17826d49d89cef21261cdc22bd27065bbc5b27d7dbf7fbbf6c",
				"zh:a5ba9945606bb7bfb821ba303957eeb40dd9ee4e706ba8da1eaf7cbeb0356e63",
				"zh:df3a5a8d6ffff7bacf19c92d10d0d500f98169ea17b3764b01a789f563d1aad7",
				"zh:086119a26576d06b8281a97e8644380da89ce16197cd955f74ea5ee664e9358b",
				"zh:1e5f7a5f3ade7b8b1d1d59c5cea2e1a2f8d2f8c3f41962dbbe8647e222be8239",
				"zh:0e9fd0f3e2254b526a0e81e0cfdfc82583b0cd343778c53ead21aa7d52f776d7",
				"zh:66a947e7de1c74caf9f584c3ed4e91d2cb1af6fe5ce8abaf1cf8f7ff626a09d1",
				"zh:def1b73849bec0dc57a04405847921bf9206c75b52ae9de195476facb26bd85e",
				"zh:48f1826ec31d6f104e46cc2022b41f30cd1019ef48eaec9697654ef9ec37a879",
				"zh:17e0b496022bc4e4137be15e96d2b051c8acd6e14cb48d9b13b262330464f6cc",
				"zh:2696c86228f491bc5425561c45904c9ce39b1c676b1e17734cb2ee6b578c4bcd",
			},
		},
	}

	for i, testCase := range testCases {
		testCase := testCase

		t.Run(fmt.Sprintf("testCase-%d", i), func(t *testing.T) {
			t.Parallel()

			auth := NewSignatureAuthentication([]byte(testCase.shasums), nil, nil)
			authWithHashes, ok := auth.(PackageAuthenticationHashes)
			require.True(t, ok)

			actualHash := authWithHashes.AcceptableHashes()
			assert.Equal(t, testCase.expectedHashes, actualHash)
		})
	}
}
