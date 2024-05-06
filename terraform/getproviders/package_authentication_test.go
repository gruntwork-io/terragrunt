package getproviders

import (
	"crypto/sha256"
	"encoding/base64"
	"errors"
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

func TestArchiveChecksumAuthentication(t *testing.T) {
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
			actualResult, actualErr := auth.Authenticate(testCase.path)
			if testCase.expectedErr != nil {
				require.EqualError(t, actualErr, testCase.expectedErr.Error())
			} else {
				require.NoError(t, actualErr)
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
			_, actualErr := auth.Authenticate(testCase.path)

			if testCase.expectedErr != nil {
				require.EqualError(t, actualErr, testCase.expectedErr.Error())
			} else {
				require.NoError(t, actualErr)
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

func TestSignatureAuthenticate(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		path           string
		document       []byte
		signature      string
		keys           []SigningKey
		expectedResult *PackageAuthenticationResult
		expectedErr    error
	}{
		{
			"testdata/my-package.zip",
			[]byte(testProviderShaSums),
			testHashicorpSignatureGoodBase64,
			[]SigningKey{{ASCIIArmor: HashicorpPublicKey}},
			NewPackageAuthenticationResult(officialProvider),
			nil,
		},
		{
			"testdata/my-package.zip",
			[]byte("example shasums data"),
			testHashicorpSignatureGoodBase64,
			[]SigningKey{{ASCIIArmor: "invalid PGP armor value"}},
			nil,
			errors.New("error decoding signing key: openpgp: invalid argument: no armored data found"),
		},
		{
			"testdata/my-package.zip",
			[]byte("example shasums data"),
			testSignatureBadBase64,
			[]SigningKey{{ASCIIArmor: testAuthorKeyArmor}},
			nil,
			errors.New("error checking signature: openpgp: invalid data: signature subpacket truncated"),
		},
		{
			"testdata/my-package.zip",
			[]byte("example shasums data"),
			testAuthorSignatureGoodBase64,
			[]SigningKey{{ASCIIArmor: HashicorpPublicKey}},
			nil,
			errors.New("authentication signature from unknown issuer"),
		},
		{
			"testdata/my-package.zip",
			[]byte("example shasums data"),
			testAuthorSignatureGoodBase64,
			[]SigningKey{{ASCIIArmor: testAuthorKeyArmor, TrustSignature: "invalid PGP armor value"}},
			nil,
			errors.New("error decoding trust signature: EOF"),
		},
		{
			"testdata/my-package.zip",
			[]byte("example shasums data"),
			testAuthorSignatureGoodBase64,
			[]SigningKey{{ASCIIArmor: testAuthorKeyArmor, TrustSignature: testOtherKeyTrustSignatureArmor}},
			nil,
			errors.New("error verifying trust signature: openpgp: invalid signature: RSA verification failure"),
		},
	}

	for i, testCase := range testCases {
		testCase := testCase

		t.Run(fmt.Sprintf("testCase-%d", i), func(t *testing.T) {
			t.Parallel()

			signature, err := base64.StdEncoding.DecodeString(testCase.signature)
			require.NoError(t, err)

			auth := NewSignatureAuthentication(testCase.document, signature, testCase.keys)
			actualResult, actualErr := auth.Authenticate(testCase.path)

			if testCase.expectedErr != nil {
				require.EqualError(t, actualErr, testCase.expectedErr.Error())
			} else {
				require.NoError(t, actualErr)
			}

			assert.Equal(t, testCase.expectedResult, actualResult)
		})
	}
}

// testHashicorpSignatureGoodBase64 is a signature of testProviderShaSums signed with
// HashicorpPublicKey, which represents the SHA256SUMS.sig file downloaded for
// an official release.
const testHashicorpSignatureGoodBase64 = `wsFcBAABCAAQBQJgga+GCRCwtEEJdoW2dgAA` +
	`o0YQAAW911BGDr2WHLo5NwcZenwHyxL5DX9g+4BknKbc/WxRC1hD8Afi3eygZk1yR6eT4Gp2H` +
	`yNOwCjGL1PTONBumMfj9udIeuX8onrJMMvjFHh+bORGxBi4FKr4V3b2ZV1IYOjWMEyyTGRDvw` +
	`SCdxBkp3apH3s2xZLmRoAj84JZ4KaxGF7hlT0j4IkNyQKd2T5cCByN9DV80+x+HtzaOieFwJL` +
	`97iyGj6aznXfKfslK6S4oIrVTwyLTrQbxSxA0LsdUjRPHnJamL3sFOG77qUEUoXG3r61yi5vW` +
	`V4P5gCH/+C+VkfGHqaB1s0jHYLxoTEXtwthe66MydDBPe2Hd0J12u9ppOIeK3leeb4uiixWIi` +
	`rNdpWyjr/LU1KKWPxsDqMGYJ9TexyWkXjEpYmIEiY1Rxar8jrLh+FqVAhxRJajjgSRu5pZj50` +
	`CNeKmmbyolLhPCmICjYYU/xKPGXSyDFqonVVyMWCSpO+8F38OmwDQHIk5AWyc8hPOAZ+g5N95` +
	`cfUAzEqlvmNvVHQIU40Y6/Ip2HZzzFCLKQkMP1aDakYHq5w4ZO/ucjhKuoh1HDQMuMnZSu4eo` +
	`2nMTBzYZnUxwtROrJZF1t103avbmP2QE/GaPvLIQn7o5WMV3ZcPCJ+szzzby7H2e33WIynrY/` +
	`95ensBxh7mGFbcQ1C59b5o7viwIaaY2`

// testSignatureBadBase64 is an invalid signature.
const testSignatureBadBase64 = `iQEzBAABCAAdFiEEW/7sQxfnRgCGIZcGN6arO88s` +
	`4qIKqL6DwddBF4Ju2svn2MeNMGfE358H31mxAl2k4PPrwBTR1sFUCUOzAXVA/g9Ov5Y9ni2G` +
	`rkTahBtV9yuUUd1D+oRTTTdP0bj3A+3xxXmKTBhRuvurydPTicKuWzeILIJkcwp7Kl5UbI2N` +
	`n1ayZdaCIw/r4w==`

// testAuthorKeyArmor is test key ID 5BFEEC4317E746008621970637A6AB3BCF2C170A.
const testAuthorKeyArmor = `-----BEGIN PGP PUBLIC KEY BLOCK-----

mQENBF5vhgYBCAC40OcC2hEx3yGiLhHMbt7DAVEQ0nZwAWy6oL98niknLumBa1VO
nMYshP+o/FKOFatBl8aXhmDo606P6pD9d4Pg/WNehqT7hGNHcAFlm+8qjQAvE5uX
Z/na/Np7dmWasCiL5hYyHEnKU/XFpc9KyicbkS7n8igP1LEb8xDD1pMLULQsQHA4
258asvtwjoYTZIij1I6bUE178bGFPNCfj+FzQM8nKzPpDVxZ7njN9c2sB9FEdJ1+
S9mZQNK5PbJuEAOpD5Jp9BnGE16jsLUhDmvGHBjFZAXMBkNSloEMHhs2ty9lEzoF
eJmJx7XCGw+ds1SWp4MsHQPWzXxAlrfa4GMlABEBAAG0R1RlcnJhZm9ybSBUZXN0
aW5nIChwbHVnaW4vZGlzY292ZXJ5LykgPHRlcnJhZm9ybSt0ZXN0aW5nQGhhc2hp
Y29ycC5jb20+iQFOBBMBCAA4FiEEW/7sQxfnRgCGIZcGN6arO88sFwoFAl5vhgYC
GwMFCwkIBwIGFQoJCAsCBBYCAwECHgECF4AACgkQN6arO88sFwpWvQf/apaMu4Bm
ea8AGjdl9acQhHBpWsyiHLIfZvN11xxN/f3+YITvPXIe2PMgveqNfXxu6PIeZGDb
0DBvnBQy/vqmA+sCQ8t8+kIWdfZ1EeM2YcXdmAEtriooLvc85JFYjafLIKSj9N7o
V/R/e1BCW/v1/7Je47c+6FSt3HHhwyT5AZ3BCq1zpw6PeCDSQ/gZr3Mvq4CjeLA/
K+8TM3KyOF4qBGDvzGzp/t9umQSS2L0ozd90lxJtf5Q8ozqDaBiDo+f/osXT2EvN
VwPP/xh/gABkXiNrPylFbeD+XPAC4N7NmYK5aPDzRYXXknP8e9PDMykoJKZ+bSdz
F3IZ4q5RDHmmNbkBDQReb4YGAQgAt15e1F8TPQQm1jK8+scypHgfmPHbp7Qsulo1
GTcUd8QmhbR4kayuLDEpJYzq6+IoTM4TPqsdVuq/1Nwey9oyK0wXk/SUR29nRIQh
3GBg7JVg1YsObsfVTvEflYOdjk8T/Udqs4I6HnmSbtzsaohzybutpWXPUkW8OzFI
ATwfVTrrz70Yxs+ly0nSEH2Yf+kg2uYZvv5KsJ3MNENhXnHnlaTy2IfhsxAX0xOG
pa9fXV3NzdEbl0mYaEzMi77qRAyIQ9VrIL5F0yY/LlbpLSl6xk2+BB2v3a1Ey6SJ
w4/le6AM0wlH2hKPCTlkvM0IvUWjlzrPzCkeu027iVc+fqdyiQARAQABiQE2BBgB
CAAgFiEEW/7sQxfnRgCGIZcGN6arO88sFwoFAl5vhgYCGwwACgkQN6arO88sFwqz
nAf/eF4oZG9F8sJX01mVdDm/L7Uthe4xjTdl7jwV4ygNX+pCyWrww3qc3qbd3QKg
CFqIt/TAPE/OxHxCFuxalQefpOqfxjKzvcktxzWmpgxaWsvHaXiS4bKBPz78N/Ke
MUtcjGHyLeSzYPUfjquqDzQxqXidRYhyHGSy9c0NKZ6wCElLZ6KcmCQb4sZxVwfu
ssjwAFbPMp1nr0f5SWCJfhTh7QF7lO2ldJaKMlcBM8aebmqFQ52P7ZWOFcgeerng
G7Zdrci1KEd943HhzDCsUFz4gJwbvUyiAYb2ddndpUBkYwCB/XrHWPOSnGxHgZoo
1gIqed9OV/+s5wKxZPjL0pCStQ==
=mYqJ
-----END PGP PUBLIC KEY BLOCK-----`

// testAuthorSignatureGoodBase64 is a signature of testShaSums signed with
// testAuthorKeyArmor, which represents the SHA256SUMS.sig file downloaded for
// a release.
const testAuthorSignatureGoodBase64 = `iQEzBAABCAAdFiEEW/7sQxfnRgCGIZcGN6arO88s` +
	`FwoFAl5vh7gACgkQN6arO88sFwrAlQf6Al77qzjxNIj+NQNJfBGYUE5jHIgcuWOs1IPRTYUI` +
	`rHQIUU2RVrdHoAefKTKNzGde653JK/pYTflSV+6ini3/aZZnXlF6t001w3wswmakdwTr0hXx` +
	`Ez/hHYio72Gpn7+T/L+nl6dKkjeGqd/Kor5x2TY9uYB737ESmAe5T8ZlPaGMFHh0mYlNTeRq` +
	`4qIKqL6DwddBF4Ju2svn2MeNMGfE358H31mxAl2k4PPrwBTR1sFUCUOzAXVA/g9Ov5Y9ni2G` +
	`rkTahBtV9yuUUd1D+oRTTTdP0bj3A+3xxXmKTBhRuvurydPTicKuWzeILIJkcwp7Kl5UbI2N` +
	`n1ayZdaCIw/r4w==`

// testOtherKeyTrustSignatureArmor is a trust signature of another key (not the
// author key), signed with HashicorpPartnersKey.
const testOtherKeyTrustSignatureArmor = `-----BEGIN PGP SIGNATURE-----

iQIzBAABCAAdFiEEUYkGV8Ws20uCMIZWfXLUJo5GYPwFAl6POvsACgkQfXLUJo5G
YPyGihAAomM1kGmrC5KRgWQ+V47r8wFoIkhsTgAYb9ENOzn/RVJt3SJSstcKxfA3
7HW5R4kqAoXH1hcPYpUcOcdeAvtZxjGRQ9JgErV8NBg6sR11aQccCzAG4Hy0hWav
/jB5NzTEX5JFEXH6WhpWI1avh0l2j6JxO1K1s+5+5PI3KbuO+XSqeZ3QmUz9FwGu
pr0J6oYcERupzrpnmgMb5fbkpHfzffR2/MOYdF9Hae4EvDS1b7tokuuKsStNnCm0
ge7PFdekwbj/OiQrQlqM1pOw2siPX3ouWCtW8oExm9tAxNw31Bn2g3oaNMkHMqJd
hlVUZlqeJMyylUat3cY7GTQONfCnoyUHe/wv8exBUbV3v2glp9y2g9i2XmXkHOrV
Z+pnNBc+jdp3a4O0Y8fXXZdjiIolZKY8BbvzheuMrQQIOmw4N3KrZbTpLKuqz8rb
h8bqUbU42oWcJmBvzF4NZ4tQ+aFHs4CbOnjfDfS14baQr2Gqo9BqTfrzS5Pbs8lq
AhY0r+zi71lQ1rBfgZfjd8zWlOzpDO//nwKhGCqYOWke/C/T6o0zxM0R4uR4zXwT
KhvXK8/kK/L8Flaxqme0d5bzXLbsMe9I6I76DY5iNhkiFnnWt4+FhGoIDR03MTKS
SnHodBLlpKLyUXi36DCDy/iKVsieqLsAdcYe0nQFuhoQcOme33A=
=aHOG
-----END PGP SIGNATURE-----`

const testProviderShaSums = `fea4227271ebf7d9e2b61b89ce2328c7262acd9fd190e1fd6d15a591abfa848e  terraform-provider-null_3.1.0_darwin_amd64.zip
9ebf4d9704faba06b3ec7242c773c0fbfe12d62db7d00356d4f55385fc69bfb2  terraform-provider-null_3.1.0_darwin_arm64.zip
a6576c81adc70326e4e1c999c04ad9ca37113a6e925aefab4765e5a5198efa7e  terraform-provider-null_3.1.0_freebsd_386.zip
5f9200bf708913621d0f6514179d89700e9aa3097c77dac730e8ba6e5901d521  terraform-provider-null_3.1.0_freebsd_amd64.zip
fc39cc1fe71234a0b0369d5c5c7f876c71b956d23d7d6f518289737a001ba69b  terraform-provider-null_3.1.0_freebsd_arm.zip
c797744d08a5307d50210e0454f91ca4d1c7621c68740441cf4579390452321d  terraform-provider-null_3.1.0_linux_386.zip
53e30545ff8926a8e30ad30648991ca8b93b6fa496272cd23b26763c8ee84515  terraform-provider-null_3.1.0_linux_amd64.zip
cecb6a304046df34c11229f20a80b24b1603960b794d68361a67c5efe58e62b8  terraform-provider-null_3.1.0_linux_arm64.zip
e1371aa1e502000d9974cfaff5be4cfa02f47b17400005a16f14d2ef30dc2a70  terraform-provider-null_3.1.0_linux_arm.zip
a8a42d13346347aff6c63a37cda9b2c6aa5cc384a55b2fe6d6adfa390e609c53  terraform-provider-null_3.1.0_windows_386.zip
02a1675fd8de126a00460942aaae242e65ca3380b5bb192e8773ef3da9073fd2  terraform-provider-null_3.1.0_windows_amd64.zip
`
