// common integration test functions
package test

import (
	"archive/zip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terragrunt/util"
	"github.com/stretchr/testify/require"
)

func testRunAllPlan(t *testing.T, args string) (string, string, string, error) {
	t.Helper()

	tmpEnvPath := copyEnvironment(t, TEST_FIXTURE_OUT_DIR)
	cleanupTerraformFolder(t, tmpEnvPath)
	testPath := util.JoinPath(tmpEnvPath, TEST_FIXTURE_OUT_DIR)

	// run plan with output directory
	stdout, stderr, err := runTerragruntCommandWithOutput(t, fmt.Sprintf("terraform run-all plan --terragrunt-non-interactive --terragrunt-log-level debug --terragrunt-working-dir %s %s", testPath, args))

	return tmpEnvPath, stdout, stderr, err
}

func runNetworkMirrorServer(t *testing.T, ctx context.Context, urlPrefix, providerDir string) *url.URL {
	mux := http.NewServeMux()

	fs := http.FileServer(http.Dir(providerDir))
	mux.Handle(urlPrefix, http.StripPrefix(urlPrefix, fs))

	ln, err := net.Listen("tcp", ":8888")
	require.NoError(t, err)

	go func() {
		server := (&http.Server{
			Addr:    ln.Addr().String(),
			Handler: mux,
		})
		server.Serve(ln)

		<-ctx.Done()
		server.Shutdown(ctx)
	}()

	return &url.URL{
		Scheme: "http",
		Host:   ln.Addr().String(),
		Path:   urlPrefix,
	}
}

type FakeProvider struct {
	RegistryName string
	Namespace    string
	Name         string
	Version      string
	PlatformOS   string
	PlatformArch string
}

func (provider *FakeProvider) archiveName() string {
	return fmt.Sprintf("terraform-provider-%s_%s_%s_%s.zip", provider.Name, provider.Version, provider.PlatformOS, provider.PlatformArch)
}

func (provider *FakeProvider) filename() string {
	return fmt.Sprintf("terraform-provider-%s_v%s_x5", provider.Name, provider.Version)
}

func (provider *FakeProvider) CreateMirror(t *testing.T, rootDir string) {
	providerDir := filepath.Join(rootDir, provider.RegistryName, provider.Namespace, provider.Name)

	err := os.MkdirAll(providerDir, os.ModePerm)
	require.NoError(t, err)

	provider.createIndexJSON(t, providerDir)
	provider.createVersionJSON(t, providerDir)
	provider.createZipArchive(t, providerDir)
}

func (provider *FakeProvider) createVersionJSON(t *testing.T, providerDir string) {
	type VersionProvider struct {
		Hashes []string `json:"hashes"`
		URL    string   `json:"url"`
	}
	type Version struct {
		Archives map[string]VersionProvider `json:"archives"`
	}

	version := &Version{Archives: make(map[string]VersionProvider)}
	filename := filepath.Join(providerDir, fmt.Sprintf("%s.json", provider.Version))
	platform := fmt.Sprintf("%s_%s", provider.PlatformOS, provider.PlatformArch)

	unmarshalFile(t, filename, version)
	version.Archives[platform] = VersionProvider{URL: provider.archiveName()}
	marshalFile(t, filename, version)
}

func (provider *FakeProvider) createIndexJSON(t *testing.T, providerDir string) {
	type Index struct {
		Versions map[string]any `json:"versions"`
	}

	index := &Index{Versions: make(map[string]any)}
	filename := filepath.Join(providerDir, "index.json")

	unmarshalFile(t, filename, index)
	index.Versions[provider.Version] = struct{}{}
	marshalFile(t, filename, index)
}

func (provider *FakeProvider) createZipArchive(t *testing.T, providerDir string) {
	file, err := os.Create(filepath.Join(providerDir, provider.filename()))
	require.NoError(t, err)
	defer func() {
		file.Close()
		require.NoError(t, os.Remove(filepath.Join(providerDir, provider.filename())))
	}()

	err = file.Sync()
	require.NoError(t, err)

	zipFile, err := os.Create(filepath.Join(providerDir, provider.archiveName()))
	require.NoError(t, err)
	defer zipFile.Close()

	zipWriter := zip.NewWriter(zipFile)
	defer zipWriter.Close()

	fileInfo, err := file.Stat()
	require.NoError(t, err)

	header, err := zip.FileInfoHeader(fileInfo)
	require.NoError(t, err)

	header.Method = zip.Deflate
	header.Name = provider.filename()

	headerWriter, err := zipWriter.CreateHeader(header)
	require.NoError(t, err)

	_, err = io.Copy(headerWriter, file)
	require.NoError(t, err)
}

func unmarshalFile(t *testing.T, filename string, dest any) {
	if !util.FileExists(filename) {
		return
	}

	data, err := os.ReadFile(filename)
	require.NoError(t, err)
	err = json.Unmarshal(data, dest)
	require.NoError(t, err)
}

func marshalFile(t *testing.T, filename string, dest any) {
	data, err := json.Marshal(dest)
	require.NoError(t, err)
	err = os.WriteFile(filename, data, 0666)
	require.NoError(t, err)
}
