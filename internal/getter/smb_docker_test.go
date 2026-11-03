//go:build docker

package getter_test

import (
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/getter"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
)

const (
	sambaImage    = "dockurr/samba:4.23.8@sha256:9e4dd0594f2f023c48b752519be2846ae193e75b7ef462c037435eac4b835dc2"
	sambaShare    = "share"
	sambaUser     = "terragrunt"
	sambaPassword = "fake-secret-samba"
)

func TestSMBClientGetterDownloadsFromSambaShare(t *testing.T) {
	t.Parallel()

	host := runSamba(t, map[string]string{
		"modules/vpc/main.tf":             "# vpc",
		"modules/vpc/nested/variables.tf": "# variables",
	})
	v := venvtest.NewOSWithEmptyEnv()

	t.Run("directory", func(t *testing.T) {
		t.Parallel()

		dst := filepath.Join(helpers.TmpDirWOSymlinks(t), "module")

		_, err := smbDownload(t, v, sambaSource(host, sambaPassword, "modules/vpc"), dst, getter.ModeAny)
		require.NoError(t, err)

		assertFileContent(t, v.FS, filepath.Join(dst, "main.tf"), "# vpc")
		assertFileContent(t, v.FS, filepath.Join(dst, "nested", "variables.tf"), "# variables")
	})

	t.Run("file", func(t *testing.T) {
		t.Parallel()

		dst := filepath.Join(helpers.TmpDirWOSymlinks(t), "main.tf")

		_, err := smbDownload(t, v, sambaSource(host, sambaPassword, "modules/vpc/main.tf"), dst, getter.ModeFile)
		require.NoError(t, err)

		assertFileContent(t, v.FS, dst, "# vpc")
	})

	t.Run("wrong password", func(t *testing.T) {
		t.Parallel()

		const wrong = "wr0ng-smb-pw"

		_, err := smbDownload(t, v, sambaSource(host, wrong, "modules/vpc"),
			filepath.Join(helpers.TmpDirWOSymlinks(t), "module"), getter.ModeDir)
		require.Error(t, err)
		assert.NotContains(t, err.Error(), wrong)
	})

	t.Run("missing path", func(t *testing.T) {
		t.Parallel()

		_, err := smbDownload(t, v, sambaSource(host, sambaPassword, "modules/missing"),
			filepath.Join(helpers.TmpDirWOSymlinks(t), "module"), getter.ModeAny)
		require.Error(t, err)
		assert.NotContains(t, err.Error(), sambaPassword)
	})
}

// runSamba starts a Samba server whose share holds files, keyed by path in
// the share, and returns the host and port it listens on.
func runSamba(t *testing.T, files map[string]string) string {
	t.Helper()

	containerFiles := make([]testcontainers.ContainerFile, 0, len(files))
	for path, content := range files {
		containerFiles = append(containerFiles, testcontainers.ContainerFile{
			Reader:            strings.NewReader(content),
			ContainerFilePath: "/storage/" + path,
			FileMode:          0o644,
		})
	}

	_, addr := helpers.RunContainer(
		t,
		sambaImage,
		445,
		testcontainers.WithEnv(map[string]string{"NAME": sambaShare, "USER": sambaUser, "PASS": sambaPassword}),
		testcontainers.WithFiles(containerFiles...),
	)

	u, err := url.Parse(addr)
	require.NoError(t, err)

	return u.Host
}

// sambaSource returns the smb source for path in the share at host,
// authenticating with password.
func sambaSource(host, password, path string) string {
	return (&url.URL{
		Scheme: "smb",
		User:   url.UserPassword(sambaUser, password),
		Host:   host,
		Path:   "/" + sambaShare + "/" + path,
	}).String()
}

// smbDownload fetches src into dst with a default client running smbclient
// through v.
func smbDownload(t *testing.T, v *venv.Venv, src, dst string, mode getter.Mode) (*getter.GetResult, error) {
	t.Helper()

	client := getter.NewClient(logger.CreateLogger(), v)

	return getter.Download(t.Context(), client, &getter.Request{Src: src, Dst: dst, GetMode: mode})
}
