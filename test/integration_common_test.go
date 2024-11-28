// common integration test functions
package test_test

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/gruntwork-io/terragrunt/cli/commands/terraform"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/pkg/log/format"
	"github.com/gruntwork-io/terragrunt/test/helpers"

	"github.com/gruntwork-io/terragrunt/util"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/NYTimes/gziphandler"
)

func getPathRelativeTo(t *testing.T, path string, basePath string) string {
	t.Helper()

	relPath, err := util.GetPathRelativeTo(path, basePath)
	require.NoError(t, err)
	return relPath
}

func getPathsRelativeTo(t *testing.T, basePath string, paths []string) []string {
	t.Helper()

	relPaths := make([]string, len(paths))

	for i, path := range paths {
		relPath, err := util.GetPathRelativeTo(path, basePath)
		require.NoError(t, err)
		relPaths[i] = relPath
	}

	return relPaths
}

func createLogger() log.Logger {
	formatter := format.NewFormatter(format.NewKeyValueFormat())
	formatter.DisableColors()

	return log.New(log.WithLevel(log.DebugLevel), log.WithFormatter(formatter))
}

func testRunAllPlan(t *testing.T, args string) (string, string, string, error) {
	t.Helper()

	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureOutDir)
	helpers.CleanupTerraformFolder(t, tmpEnvPath)
	testPath := util.JoinPath(tmpEnvPath, testFixtureOutDir)

	// run plan with output directory
	stdout, stderr, err := helpers.RunTerragruntCommandWithOutput(t, fmt.Sprintf("terraform run-all plan --terragrunt-non-interactive --terragrunt-log-level trace --terragrunt-working-dir %s %s", testPath, args))

	return tmpEnvPath, stdout, stderr, err
}

func runNetworkMirrorServer(t *testing.T, ctx context.Context, urlPrefix, providerDir, token string) *url.URL {
	t.Helper()

	serverTLSConf, clientTLSConf := certSetup(t)

	http.DefaultTransport = &http.Transport{
		TLSClientConfig: clientTLSConf,
	}

	mux := http.NewServeMux()

	fs := http.FileServer(http.Dir(providerDir))

	withGz := gziphandler.GzipHandler(http.StripPrefix(urlPrefix, fs))

	mux.HandleFunc(urlPrefix, func(resp http.ResponseWriter, req *http.Request) {
		if token != "" {
			authHeaders := req.Header.Values("Authorization")
			assert.Contains(t, authHeaders, "Bearer "+token)
		}

		withGz.ServeHTTP(resp, req)
	})

	ln, err := tls.Listen("tcp", "localhost:8888", serverTLSConf)
	require.NoError(t, err)

	server := &http.Server{
		Addr:    ln.Addr().String(),
		Handler: mux,
	}

	go func() {
		server.Serve(ln)
	}()
	go func() {
		<-ctx.Done()
		server.Shutdown(ctx)
	}()

	return &url.URL{
		Scheme: "https",
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
	t.Helper()

	providerDir := filepath.Join(rootDir, provider.RegistryName, provider.Namespace, provider.Name)

	err := os.MkdirAll(providerDir, os.ModePerm)
	require.NoError(t, err)

	provider.createIndexJSON(t, providerDir)
	provider.createVersionJSON(t, providerDir)
	provider.createZipArchive(t, providerDir)
}

func (provider *FakeProvider) createVersionJSON(t *testing.T, providerDir string) {
	t.Helper()

	type VersionProvider struct {
		Hashes []string `json:"hashes"`
		URL    string   `json:"url"`
	}
	type Version struct {
		Archives map[string]VersionProvider `json:"archives"`
	}

	version := &Version{Archives: make(map[string]VersionProvider)}
	filename := filepath.Join(providerDir, provider.Version+".json")
	platform := fmt.Sprintf("%s_%s", provider.PlatformOS, provider.PlatformArch)

	unmarshalFile(t, filename, version)
	version.Archives[platform] = VersionProvider{URL: provider.archiveName()}
	marshalFile(t, filename, version)
}

func (provider *FakeProvider) createIndexJSON(t *testing.T, providerDir string) {
	t.Helper()

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
	t.Helper()

	file, err := os.Create(filepath.Join(providerDir, provider.filename()))
	require.NoError(t, err)
	defer func() {
		file.Close()
		require.NoError(t, os.Remove(filepath.Join(providerDir, provider.filename())))
	}()

	err = file.Truncate(1e7)
	require.NoError(t, err)

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
	t.Helper()

	if !util.FileExists(filename) {
		return
	}

	data, err := os.ReadFile(filename)
	require.NoError(t, err)
	err = json.Unmarshal(data, dest)
	require.NoError(t, err)
}

func marshalFile(t *testing.T, filename string, dest any) {
	t.Helper()

	data, err := json.Marshal(dest)
	require.NoError(t, err)
	err = os.WriteFile(filename, data, 0666)
	require.NoError(t, err)
}

func certSetup(t *testing.T) (*tls.Config, *tls.Config) {
	t.Helper()

	// set up our CA certificate
	serialNumber, err := strconv.ParseInt(time.Now().Format("20060102150405"), 10, 64)
	require.NoError(t, err)

	ca := &x509.Certificate{
		SerialNumber: big.NewInt(serialNumber),
		Subject: pkix.Name{
			Organization:  []string{"Company, INC."},
			Country:       []string{"US"},
			Province:      []string{""},
			Locality:      []string{"San Francisco"},
			StreetAddress: []string{"Golden Gate Bridge"},
			PostalCode:    []string{"94016"},
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().AddDate(10, 0, 0),
		IsCA:                  true,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth},
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		BasicConstraintsValid: true,
	}

	// create our private and public key
	caPrivKey, err := rsa.GenerateKey(rand.Reader, 4096)
	require.NoError(t, err)

	// create the CA
	caBytes, err := x509.CreateCertificate(rand.Reader, ca, ca, &caPrivKey.PublicKey, caPrivKey)
	require.NoError(t, err)

	// pem encode
	caPEM := new(bytes.Buffer)
	pem.Encode(caPEM, &pem.Block{
		Type:  "CERTIFICATE",
		Bytes: caBytes,
	})

	caPrivKeyPEM := new(bytes.Buffer)
	pem.Encode(caPrivKeyPEM, &pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(caPrivKey),
	})

	// set up our server certificate
	cert := &x509.Certificate{
		SerialNumber: big.NewInt(serialNumber),
		Subject: pkix.Name{
			Organization:  []string{"Company, INC."},
			Country:       []string{"US"},
			Province:      []string{""},
			Locality:      []string{"San Francisco"},
			StreetAddress: []string{"Golden Gate Bridge"},
			PostalCode:    []string{"94016"},
		},
		IPAddresses:  []net.IP{net.IPv4(127, 0, 0, 1), net.IPv6loopback},
		NotBefore:    time.Now(),
		NotAfter:     time.Now().AddDate(10, 0, 0),
		SubjectKeyId: []byte{1, 2, 3, 4, 6},
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth},
		KeyUsage:     x509.KeyUsageDigitalSignature,
	}

	certPrivKey, err := rsa.GenerateKey(rand.Reader, 4096)
	require.NoError(t, err)

	certBytes, err := x509.CreateCertificate(rand.Reader, cert, ca, &certPrivKey.PublicKey, caPrivKey)
	require.NoError(t, err)

	certPEM := new(bytes.Buffer)
	pem.Encode(certPEM, &pem.Block{
		Type:  "CERTIFICATE",
		Bytes: certBytes,
	})

	certPrivKeyPEM := new(bytes.Buffer)
	pem.Encode(certPrivKeyPEM, &pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(certPrivKey),
	})

	serverCert, err := tls.X509KeyPair(certPEM.Bytes(), certPrivKeyPEM.Bytes())
	require.NoError(t, err)

	serverTLSConf := &tls.Config{
		Certificates: []tls.Certificate{serverCert},
	}

	certpool := x509.NewCertPool()
	certpool.AppendCertsFromPEM(caPEM.Bytes())
	clientTLSConf := &tls.Config{
		RootCAs:            certpool,
		InsecureSkipVerify: true,
	}

	return serverTLSConf, clientTLSConf
}

func validateOutput(t *testing.T, outputs map[string]helpers.TerraformOutput, key string, value interface{}) {
	t.Helper()
	output, hasPlatform := outputs[key]
	assert.Truef(t, hasPlatform, "Expected output %s to be defined", key)
	assert.Equalf(t, output.Value, value, "Expected output %s to be %t", key, value)
}

// wrappedBinary - return which binary will be wrapped by Terragrunt, useful in CICD to run same tests against tofu and terraform
func wrappedBinary() string {
	value, found := os.LookupEnv("TERRAGRUNT_TFPATH")
	if !found {
		// if env variable is not defined, try to check through executing command
		if util.IsCommandExecutable(helpers.TofuBinary, "-version") {
			return helpers.TofuBinary
		}
		return helpers.TerraformBinary
	}
	return filepath.Base(value)
}

// expectedWrongCommandErr - return expected error message for wrong command
func expectedWrongCommandErr(command string) error {
	if wrappedBinary() == helpers.TofuBinary {
		return terraform.WrongTofuCommand(command)
	}
	return terraform.WrongTerraformCommand(command)
}

func isTerraform() bool {
	return wrappedBinary() == helpers.TerraformBinary
}

func findFilesWithExtension(dir string, ext string) ([]string, error) {
	var files []string
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() && filepath.Ext(path) == ext {
			files = append(files, path)
		}
		return nil
	})

	return files, err
}
