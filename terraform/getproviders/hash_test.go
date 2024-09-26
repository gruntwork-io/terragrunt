package getproviders_test

import (
	"fmt"
	"os"
	"testing"

	"github.com/gruntwork-io/terragrunt/terraform/getproviders"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func createFakeZipArchive(t *testing.T, content []byte) string {
	t.Helper()

	file, err := os.CreateTemp("", "*")
	require.NoError(t, err)
	defer file.Close()

	_, err = file.Write(content)
	require.NoError(t, err)

	return file.Name()
}

func TestPackageHashLegacyZipSHA(t *testing.T) {
	t.Parallel()

	tc := []struct {
		path         string
		expectedHash getproviders.Hash
	}{
		{
			createFakeZipArchive(t, []byte("1234567890")),
			"zh:c775e7b757ede630cd0aa1113bd102661ab38829ca52a6422ab782862f268646",
		},
		{
			createFakeZipArchive(t, []byte("0987654321")),
			"zh:17756315ebd47b7110359fc7b168179bf6f2df3646fcc888bc8aa05c78b38ac1",
		},
	}

	for i, tt := range tc {
		tt := tt

		t.Run(fmt.Sprintf("testCase-%d", i), func(t *testing.T) {
			t.Parallel()

			hash, err := getproviders.PackageHashLegacyZipSHA(tt.path)
			require.NoError(t, err)

			assert.Equal(t, tt.expectedHash, hash)
		})
	}
}
