package helpers

import (
	"crypto/x509"
	"errors"
	"fmt"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/aws/smithy-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNestUnder(t *testing.T) {
	t.Parallel()

	root := filepath.Join("tmp", "root")
	sep := string(filepath.Separator)

	testCases := []struct {
		name string
		path string
		want string
	}{
		{
			name: "relative path",
			path: filepath.Join("fixtures", "unit"),
			want: filepath.Join(root, "fixtures", "unit"),
		},
		{
			name: "rooted path",
			path: sep + filepath.Join("abs", "unit"),
			want: filepath.Join(root, "abs", "unit"),
		},
	}

	if runtime.GOOS == "windows" {
		testCases = append(testCases, struct {
			name string
			path string
			want string
		}{
			name: "drive letter is dropped",
			path: `C:\abs\unit`,
			want: filepath.Join(root, "abs", "unit"),
		})
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tc.want, nestUnder(root, tc.path))
		})
	}
}

func TestIsAWSResourceNotFoundError(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		err  error
		name string
		want bool
	}{
		{
			name: "missing bucket",
			err:  &smithy.GenericAPIError{Code: "NoSuchBucket"},
			want: true,
		},
		{
			name: "head request not found",
			err:  &smithy.GenericAPIError{Code: "NotFound"},
			want: true,
		},
		{
			name: "missing dynamodb table",
			err:  &smithy.GenericAPIError{Code: "ResourceNotFoundException"},
			want: true,
		},
		{
			name: "wrapped not found",
			err:  fmt.Errorf("deleting bucket: %w", &smithy.GenericAPIError{Code: "NoSuchBucket"}),
			want: true,
		},
		{
			name: "other API error",
			err:  &smithy.GenericAPIError{Code: "AccessDenied"},
			want: false,
		},
		{
			name: "not an API error",
			err:  errors.New("NoSuchBucket"),
			want: false,
		},
		{
			name: "nil",
			want: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tc.want, isAWSResourceNotFoundError(tc.err))
		})
	}
}

func TestCertSetup(t *testing.T) {
	t.Parallel()

	serverConf, clientConf := certSetup(t)

	require.Len(t, serverConf.Certificates, 1)
	require.NotEmpty(t, serverConf.Certificates[0].Certificate)
	require.NotNil(t, clientConf.RootCAs)

	leaf, err := x509.ParseCertificate(serverConf.Certificates[0].Certificate[0])
	require.NoError(t, err)

	// The leaf is signed by the CA the client trusts and is valid for the mirror's loopback address.
	_, err = leaf.Verify(x509.VerifyOptions{
		Roots:     clientConf.RootCAs,
		KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	})
	require.NoError(t, err)
	require.NoError(t, leaf.VerifyHostname("127.0.0.1"))
	assert.False(t, leaf.IsCA)
}
