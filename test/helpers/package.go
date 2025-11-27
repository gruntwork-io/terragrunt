// Package helpers provides helper functions for tests.
package helpers

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
	"io/fs"
	"math/big"
	mathRand "math/rand"
	"net"
	"net/http"
	"net/url"
	"os/exec"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gruntwork-io/terragrunt/internal/awshelper"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/pkg/log/format"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/mattn/go-shellwords"

	"os"
	"path/filepath"
	"testing"

	"github.com/NYTimes/gziphandler"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/smithy-go"
	"github.com/gruntwork-io/go-commons/version"
	"github.com/gruntwork-io/terragrunt/cli"
	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/internal/runner/run"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/util"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	TerraformFolder = ".terraform"

	TerraformState = "terraform.tfstate"

	TerraformRemoteStateS3Region = "us-west-2"

	TerraformStateBackup = "terraform.tfstate.backup"
	TerragruntCache      = ".terragrunt-cache"

	TerraformBinary = "terraform"
	TofuBinary      = "tofu"

	TerragruntDebugFile = "terragrunt-debug.tfvars.json"

	// Repeated right now, but it might not be later.
	TestFixtureOutDir = "fixtures/out-dir"

	readPermissions      = 0444
	readWritePermissions = 0666
	allPermissions       = 0777

	caKeyBits = 4096

	semverPartsLen = 3
)

type TerraformOutput struct {
	Type      any  `json:"Type"`
	Value     any  `json:"Value"`
	Sensitive bool `json:"Sensitive"`
}

func CopyEnvironment(t *testing.T, environmentPath string, includeInCopy ...string) string {
	t.Helper()

	tmpDir := t.TempDir()

	t.Logf("Copying %s to %s", environmentPath, tmpDir)

	require.NoError(
		t,
		util.CopyFolderContents(logger.CreateLogger(), environmentPath, util.JoinPath(tmpDir, environmentPath), ".terragrunt-test", includeInCopy, nil),
	)

	tmpDir, err := filepath.EvalSymlinks(tmpDir)
	require.NoError(t, err)

	return tmpDir
}

func CreateTmpTerragruntConfig(t *testing.T, templatesPath string, s3BucketName string, lockTableName string, configFileName string) string {
	t.Helper()

	tmpFolder := t.TempDir()

	tmpTerragruntConfigFile := util.JoinPath(tmpFolder, configFileName)
	originalTerragruntConfigPath := util.JoinPath(templatesPath, configFileName)
	CopyTerragruntConfigAndFillPlaceholders(t, originalTerragruntConfigPath, tmpTerragruntConfigFile, s3BucketName, lockTableName, "not-used")

	return tmpTerragruntConfigFile
}

func CreateTmpTerragruntConfigContent(t *testing.T, contents string, configFileName string) string {
	t.Helper()

	tmpFolder := t.TempDir()

	tmpTerragruntConfigFile := util.JoinPath(tmpFolder, configFileName)

	if err := os.WriteFile(tmpTerragruntConfigFile, []byte(contents), readPermissions); err != nil {
		t.Fatalf("Error writing temp Terragrunt config to %s: %v", tmpTerragruntConfigFile, err)
	}

	return tmpTerragruntConfigFile
}

func CopyTerragruntConfigAndFillPlaceholders(t *testing.T, configSrcPath string, configDestPath string, s3BucketName string, lockTableName string, region string) {
	t.Helper()

	CopyAndFillMapPlaceholders(t, configSrcPath, configDestPath, map[string]string{
		"__FILL_IN_BUCKET_NAME__":      s3BucketName,
		"__FILL_IN_LOCK_TABLE_NAME__":  lockTableName,
		"__FILL_IN_REGION__":           region,
		"__FILL_IN_LOGS_BUCKET_NAME__": s3BucketName + "-tf-state-logs",
	})
}

func CopyAndFillMapPlaceholders(t *testing.T, srcPath string, destPath string, placeholders map[string]string) {
	t.Helper()

	contents, err := util.ReadFileAsString(srcPath)
	require.NoError(t, err, "Error reading file at %s: %v", srcPath, err)

	// iterate over placeholders and replace placeholders
	for k, v := range placeholders {
		contents = strings.ReplaceAll(contents, k, v)
	}

	err = os.WriteFile(destPath, []byte(contents), readPermissions)
	require.NoError(t, err, "Error writing temp file to %s: %v", destPath, err)
}

// UniqueID returns a unique (ish) id we can attach to resources and tfstate files so they don't conflict with each other
// Uses base 62 to generate a 6 character string that's unlikely to collide with the handful of tests we run in
// parallel. Based on code here: http://stackoverflow.com/a/9543797/483528
func UniqueID() string {
	const (
		base62Chars    = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"
		uniqueIDLength = 6 // Should be good for 62^6 = 56+ billion combinations
	)

	var out bytes.Buffer

	for range uniqueIDLength {
		out.WriteByte(base62Chars[mathRand.Intn(len(base62Chars))])
	}

	return out.String()
}

// CreateS3ClientForTest creates a S3 client we can use at test time. If there are any errors creating the client, fail the test.
func CreateS3ClientForTest(t *testing.T, awsRegion string, opts ...options.TerragruntOptionsFunc) *s3.Client {
	t.Helper()

	mockOptions, err := options.NewTerragruntOptionsForTest("aws_s3_test")
	require.NoError(t, err, "Error creating mockOptions")

	for _, opt := range opts {
		opt(mockOptions)
	}

	awsConfig := &awshelper.AwsSessionConfig{Region: awsRegion}

	cfg, err := awshelper.CreateAwsConfig(t.Context(), logger.CreateLogger(), awsConfig, mockOptions)
	require.NoError(t, err, "Error creating S3 client")

	return s3.NewFromConfig(cfg)
}

// CreateDynamoDBClientForTest creates a DynamoDB client we can use at test time. If there are any errors creating the client, fail the test.
func CreateDynamoDBClientForTest(t *testing.T, awsRegion, awsProfile, iamRoleArn string) *dynamodb.Client {
	t.Helper()

	mockOptions, err := options.NewTerragruntOptionsForTest("aws_dynamodb_test")
	require.NoError(t, err, "Error creating mockOptions")

	sessionConfig := &awshelper.AwsSessionConfig{
		Region:  awsRegion,
		Profile: awsProfile,
		RoleArn: iamRoleArn,
	}

	cfg, err := awshelper.CreateAwsConfig(t.Context(), logger.CreateLogger(), sessionConfig, mockOptions)
	require.NoError(t, err, "Error creating DynamoDB client")

	return dynamodb.NewFromConfig(cfg)
}

// DeleteS3Bucket deletes the specified S3 bucket potentially with error to clean up after a test.
func DeleteS3Bucket(t *testing.T, awsRegion string, bucketName string, opts ...options.TerragruntOptionsFunc) error {
	t.Helper()

	client := CreateS3ClientForTest(t, awsRegion, opts...)

	t.Logf("Deleting test s3 bucket %s", bucketName)

	// First check if bucket exists
	_, err := client.HeadBucket(t.Context(), &s3.HeadBucketInput{Bucket: aws.String(bucketName)})
	if err != nil {
		if isAWSResourceNotFoundError(err) {
			t.Logf("S3 bucket %s does not exist, cleanup already complete", bucketName)
			return nil
		}

		t.Logf("Error checking if S3 bucket %s exists: %v", bucketName, err)
	}

	cleanS3Bucket(t, client, bucketName)

	if _, err := client.DeleteBucket(t.Context(), &s3.DeleteBucketInput{Bucket: aws.String(bucketName)}); err != nil {
		if isAWSResourceNotFoundError(err) {
			t.Logf("S3 bucket %s was already deleted", bucketName)
			return nil
		}

		t.Logf("Failed to delete S3 bucket %s: %v", bucketName, err)

		// If the bucket is not empty, try to clean it again before deleting it.
		// This is a workaround for a race condition in eventual consistency.
		// Sleep for a little bit first to give the bucket a chance to be ready.
		time.Sleep(1 * time.Second)

		cleanS3Bucket(t, client, bucketName)

		if _, err = client.DeleteBucket(t.Context(), &s3.DeleteBucketInput{Bucket: aws.String(bucketName)}); err != nil {
			if isAWSResourceNotFoundError(err) {
				t.Logf("S3 bucket %s was already deleted", bucketName)
				return nil
			}

			t.Logf("Failed to delete S3 bucket %s: %v", bucketName, err)

			return err
		}

		return err
	}

	return nil
}

func cleanS3Bucket(t *testing.T, client *s3.Client, bucketName string) {
	t.Helper()

	versionsInput := &s3.ListObjectVersionsInput{
		Bucket: aws.String(bucketName),
	}

	for {
		out, err := client.ListObjectVersions(t.Context(), versionsInput)
		if err != nil {
			if isAWSResourceNotFoundError(err) {
				t.Logf("S3 bucket %s does not exist, skipping cleanup", bucketName)
				return
			}

			require.NoError(t, err)
		}

		if len(out.Versions) == 0 && len(out.DeleteMarkers) == 0 {
			break
		}

		if len(out.Versions) > 0 {
			var objectsToDelete []s3types.ObjectIdentifier

			for _, version := range out.Versions {
				objectsToDelete = append(objectsToDelete, s3types.ObjectIdentifier{
					Key:       version.Key,
					VersionId: version.VersionId,
				})
			}

			deleteInput := &s3.DeleteObjectsInput{
				Bucket: aws.String(bucketName),
				Delete: &s3types.Delete{
					Objects: objectsToDelete,
				},
			}

			_, err := client.DeleteObjects(t.Context(), deleteInput)
			if err != nil {
				if isAWSResourceNotFoundError(err) {
					t.Logf("S3 bucket %s was deleted during cleanup", bucketName)
					return
				}

				require.NoError(t, err)
			}
		}

		if len(out.DeleteMarkers) > 0 {
			var objectsToDelete []s3types.ObjectIdentifier

			for _, marker := range out.DeleteMarkers {
				objectsToDelete = append(objectsToDelete, s3types.ObjectIdentifier{
					Key:       marker.Key,
					VersionId: marker.VersionId,
				})
			}

			deleteInput := &s3.DeleteObjectsInput{
				Bucket: aws.String(bucketName),
				Delete: &s3types.Delete{
					Objects: objectsToDelete,
				},
			}

			_, err := client.DeleteObjects(t.Context(), deleteInput)
			if err != nil {
				if isAWSResourceNotFoundError(err) {
					t.Logf("S3 bucket %s was deleted during cleanup", bucketName)
					return
				}

				require.NoError(t, err)
			}
		}

		if out.IsTruncated == nil || !*out.IsTruncated {
			break
		}

		versionsInput.KeyMarker = out.NextKeyMarker
		versionsInput.VersionIdMarker = out.NextVersionIdMarker
	}
}

func FileIsInFolder(t *testing.T, name string, path string) bool {
	t.Helper()

	found := false
	err := filepath.Walk(path, func(path string, info os.FileInfo, err error) error {
		require.NoError(t, err)

		if filepath.Base(path) == name {
			found = true
		}

		return nil
	})

	require.NoError(t, err)

	return found
}

func RunValidateAllWithIncludeAndGetIncludedModules(t *testing.T, rootModulePath string, includeModulePaths []string, strictInclude bool) []string {
	t.Helper()

	cmdParts := []string{
		"terragrunt", "run", "--all", "validate",
		"--non-interactive",
		"--log-level", "debug",
		"--working-dir", rootModulePath,
	}

	for _, module := range includeModulePaths {
		cmdParts = append(cmdParts, "--queue-include-dir", module)
	}

	if strictInclude {
		cmdParts = append(cmdParts, "--queue-strict-include")
	}

	cmd := strings.Join(cmdParts, " ")

	validateAllStdout := bytes.Buffer{}
	validateAllStderr := bytes.Buffer{}
	err := RunTerragruntCommand(
		t,
		cmd,
		&validateAllStdout,
		&validateAllStderr,
	)

	LogBufferContentsLineByLine(t, validateAllStdout, "run --all validate stdout")
	LogBufferContentsLineByLine(t, validateAllStderr, "run --all validate stderr")

	require.NoError(t, err)

	require.NoError(t, err)

	includedModulesRegexp, err := regexp.Compile(`=> Unit (.+) \(excluded: (true|false)`)
	require.NoError(t, err)

	matches := includedModulesRegexp.FindAllStringSubmatch(validateAllStderr.String(), -1)
	includedModules := []string{}

	for _, match := range matches {
		if match[2] == "false" {
			includedModules = append(includedModules, GetPathRelativeTo(t, match[1], rootModulePath))
		}
	}

	sort.Strings(includedModules)

	return includedModules
}

func GetPathRelativeTo(t *testing.T, path string, basePath string) string {
	t.Helper()

	relPath, err := util.GetPathRelativeTo(path, basePath)
	require.NoError(t, err)

	return relPath
}

func GetPathsRelativeTo(t *testing.T, basePath string, paths []string) []string {
	t.Helper()

	relPaths := make([]string, len(paths))

	for i, path := range paths {
		relPath, err := util.GetPathRelativeTo(path, basePath)
		require.NoError(t, err)

		relPaths[i] = relPath
	}

	return relPaths
}

func TestRunAllPlan(t *testing.T, args string) (string, string, string, error) {
	t.Helper()

	tmpEnvPath := CopyEnvironment(t, TestFixtureOutDir)
	CleanupTerraformFolder(t, tmpEnvPath)
	testPath := util.JoinPath(tmpEnvPath, TestFixtureOutDir)

	// run plan with output directory
	stdout, stderr, err := RunTerragruntCommandWithOutput(t, fmt.Sprintf("terraform run --all plan --non-interactive --log-level trace --working-dir %s %s", testPath, args))

	return tmpEnvPath, stdout, stderr, err
}

func RunNetworkMirrorServer(t *testing.T, ctx context.Context, urlPrefix, providerDir, token string) *url.URL {
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
		err := server.Serve(ln)
		assert.NoError(t, err)
	}()

	go func() {
		<-ctx.Done()
		err := server.Shutdown(ctx)
		assert.NoError(t, err)
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
		URL    string   `json:"url"`
		Hashes []string `json:"hashes"`
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

	// I wouldn't ignore this lint, but I actually don't know what
	// the number is there for.
	err = file.Truncate(1e7) //nolint:mnd
	require.NoError(t, err)

	err = file.Sync()
	require.NoError(t, err)

	zipFile, err := os.Create(filepath.Join(providerDir, provider.archiveName()))
	require.NoError(t, err)

	defer zipFile.Close()

	zipWriter := zip.NewWriter(zipFile)
	defer require.NoError(t, zipWriter.Close())

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
	err = os.WriteFile(filename, data, readWritePermissions)
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
		NotAfter:              time.Now().AddDate(10, 0, 0), //nolint:mnd
		IsCA:                  true,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth},
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		BasicConstraintsValid: true,
	}

	// create our private and public key
	caPrivKey, err := rsa.GenerateKey(rand.Reader, caKeyBits)
	require.NoError(t, err)

	// create the CA
	caBytes, err := x509.CreateCertificate(rand.Reader, ca, ca, &caPrivKey.PublicKey, caPrivKey)
	require.NoError(t, err)

	// pem encode
	caPEM := new(bytes.Buffer)
	require.NoError(t, pem.Encode(caPEM, &pem.Block{
		Type:  "CERTIFICATE",
		Bytes: caBytes,
	}))

	caPrivKeyPEM := new(bytes.Buffer)
	require.NoError(t, pem.Encode(caPrivKeyPEM, &pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(caPrivKey),
	}))

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
		IPAddresses:  []net.IP{net.IPv4(127, 0, 0, 1), net.IPv6loopback}, //nolint:mnd
		NotBefore:    time.Now(),
		NotAfter:     time.Now().AddDate(10, 0, 0), //nolint:mnd
		SubjectKeyId: []byte{1, 2, 3, 4, 6},
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth},
		KeyUsage:     x509.KeyUsageDigitalSignature,
	}

	certPrivKey, err := rsa.GenerateKey(rand.Reader, caKeyBits)
	require.NoError(t, err)

	certBytes, err := x509.CreateCertificate(rand.Reader, cert, ca, &certPrivKey.PublicKey, caPrivKey)
	require.NoError(t, err)

	certPEM := new(bytes.Buffer)
	require.NoError(t, pem.Encode(certPEM, &pem.Block{
		Type:  "CERTIFICATE",
		Bytes: certBytes,
	}))

	certPrivKeyPEM := new(bytes.Buffer)
	require.NoError(t, pem.Encode(certPrivKeyPEM, &pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(certPrivKey),
	}))

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

func ValidateOutput(t *testing.T, outputs map[string]TerraformOutput, key string, value any) {
	t.Helper()

	output, hasPlatform := outputs[key]

	assert.Truef(t, hasPlatform, "Expected output %s to be defined", key)
	assert.Equalf(t, output.Value, value, "Expected output %s to be %t", key, value)
}

// WrappedBinary - return which binary will be wrapped by Terragrunt, useful in CICD to run same tests against tofu and terraform
func WrappedBinary() string {
	value, found := os.LookupEnv("TG_TF_PATH")
	if !found {
		// if env variable is not defined, try to check through executing command
		if util.IsCommandExecutable(context.Background(), TofuBinary, "-version") {
			return TofuBinary
		}

		return TerraformBinary
	}

	return filepath.Base(value)
}

// ExpectedWrongCommandErr - return expected error message for wrong command
func ExpectedWrongCommandErr(command string) error {
	if WrappedBinary() == TofuBinary {
		return run.WrongTofuCommand(command)
	}

	return run.WrongTerraformCommand(command)
}

// IsTerraform checks if the wrapped binary currently in use is the Terraform binary.
func IsTerraform() bool {
	return WrappedBinary() == TerraformBinary
}

// IsTerraform110OrHigher checks if the installed Terraform binary is version 1.10.0 or higher.
func IsTerraform110OrHigher(t *testing.T) bool {
	t.Helper()

	const (
		requiredMajor = 1
		requiredMinor = 10
	)

	if !IsTerraform() {
		return false
	}

	output, err := exec.CommandContext(t.Context(), WrappedBinary(), "-version").Output()
	require.NoError(t, err)

	matches := regexp.MustCompile(`Terraform v(\d+)\.(\d+)\.`).FindStringSubmatch(string(output))
	require.Len(t, matches, semverPartsLen, "Expected Terraform version to be in the format 'Terraform v1.10.0'")

	major, err := strconv.Atoi(matches[1])
	require.NoError(t, err)

	minor, err := strconv.Atoi(matches[2])
	require.NoError(t, err)

	return major > requiredMajor || (major == requiredMajor && minor >= requiredMinor)
}

// IsOpenTofuInstalled checks if OpenTofu is installed.
func IsOpenTofuInstalled() bool {
	return util.IsCommandExecutable(context.Background(), TofuBinary, "-version")
}

// IsTerraformInstalled checks if Terraform is installed.
func IsTerraformInstalled() bool {
	return util.IsCommandExecutable(context.Background(), TerraformBinary, "-version")
}

// IsNativeS3LockingSupported checks if the installed Terraform binary supports native S3 locking.
// This is the case when using Terraform 1.11 or higher, or using OpenTofu 1.10 or higher.
func IsNativeS3LockingSupported(t *testing.T) bool {
	t.Helper()

	const (
		terraformRequiredMajor = 1
		terraformRequiredMinor = 11
		tofuRequiredMajor      = 1
		tofuRequiredMinor      = 10
	)

	if IsTerraform() {
		output, err := exec.CommandContext(t.Context(), TerraformBinary, "-version").Output()
		require.NoError(t, err)

		matches := regexp.MustCompile(`Terraform v(\d+)\.(\d+)\.`).FindStringSubmatch(string(output))
		require.Len(t, matches, semverPartsLen, "Expected Terraform version to be in the format 'Terraform v1.10.0'")

		major, err := strconv.Atoi(matches[1])
		require.NoError(t, err)

		minor, err := strconv.Atoi(matches[2])
		require.NoError(t, err)

		return major > terraformRequiredMajor || (major == terraformRequiredMajor && minor >= terraformRequiredMinor)
	}

	output, err := exec.CommandContext(t.Context(), TofuBinary, "-version").Output()
	require.NoError(t, err)

	matches := regexp.MustCompile(`OpenTofu v(\d+)\.(\d+)\.`).FindStringSubmatch(string(output))
	require.Len(t, matches, semverPartsLen, "Expected OpenTofu version to be in the format 'OpenTofu v1.10.0'")

	major, err := strconv.Atoi(matches[1])
	require.NoError(t, err)

	minor, err := strconv.Atoi(matches[2])
	require.NoError(t, err)

	return major > tofuRequiredMajor || (major == tofuRequiredMajor && minor >= tofuRequiredMinor)
}

func FindFilesWithExtension(dir string, ext string) ([]string, error) {
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

func CleanupTerraformFolder(t *testing.T, templatesPath string) {
	t.Helper()

	RemoveFile(t, util.JoinPath(templatesPath, TerraformState))
	RemoveFile(t, util.JoinPath(templatesPath, TerraformStateBackup))
	RemoveFile(t, util.JoinPath(templatesPath, TerragruntDebugFile))
	RemoveFolder(t, util.JoinPath(templatesPath, TerraformFolder))
}

func CleanupTerragruntFolder(t *testing.T, templatesPath string) {
	t.Helper()

	RemoveFolder(t, util.JoinPath(templatesPath, TerragruntCache))
}

func RemoveFile(t *testing.T, path string) {
	t.Helper()

	if util.FileExists(path) {
		if err := os.Remove(path); err != nil {
			t.Fatalf("Error while removing %s: %v", path, err)
		}
	}
}

func RemoveFolder(t *testing.T, path string) {
	t.Helper()

	if util.FileExists(path) {
		if err := os.RemoveAll(path); err != nil {
			t.Fatalf("Error while removing %s: %v", path, err)
		}
	}
}

func RunTerragruntCommandWithContext(
	t *testing.T,
	ctx context.Context,
	command string,
	writer,
	errwriter io.Writer,
	extraArgs ...string,
) error {
	t.Helper()

	parser := shellwords.NewParser()

	args, err := parser.Parse(command)
	require.NoError(t, err)

	if !strings.Contains(command, "-log-format") && !strings.Contains(command, "-log-custom-format") {
		var builtinCmd []string

		for i := range args {
			if args[i] == "--" {
				builtinCmd = make([]string, len(args[i:]))
				copy(builtinCmd, args[i:])
				args = args[:i]

				break
			}
		}

		args = append(append(args, "--log-format=key-value"), builtinCmd...)
	}

	t.Log(args)

	opts := options.NewTerragruntOptionsWithWriters(writer, errwriter)

	l := log.New(
		log.WithOutput(errwriter),
		log.WithLevel(options.DefaultLogLevel),
		log.WithFormatter(format.NewFormatter(format.NewPrettyFormatPlaceholders())),
	)

	app := cli.NewApp(l, opts) //nolint:contextcheck

	ctx = log.ContextWithLogger(ctx, l)

	return app.RunContext(ctx, args)
}

func RunTerragruntCommand(t *testing.T, command string, writer io.Writer, errwriter io.Writer) error {
	t.Helper()

	return RunTerragruntCommandWithContext(t, t.Context(), command, writer, errwriter)
}

func RunTerragruntVersionCommand(t *testing.T, ver string, command string, writer io.Writer, errwriter io.Writer) error {
	t.Helper()

	version.Version = ver

	return RunTerragruntCommand(t, command, writer, errwriter)
}

func RunTerragrunt(t *testing.T, command string) {
	t.Helper()

	RunTerragruntRedirectOutput(t, command, os.Stdout, os.Stderr)
}

func LogBufferContentsLineByLine(t *testing.T, out bytes.Buffer, label string) {
	t.Helper()
	t.Logf("[%s] Full contents of %s:", t.Name(), label)

	lines := strings.SplitSeq(out.String(), "\n")
	for line := range lines {
		t.Logf("[%s] %s", t.Name(), line)
	}
}

func RunTerragruntCommandWithOutputWithContext(t *testing.T, ctx context.Context, command string) (string, string, error) {
	t.Helper()

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}
	err := RunTerragruntCommandWithContext(t, ctx, command, &stdout, &stderr)
	LogBufferContentsLineByLine(t, stdout, "stdout")
	LogBufferContentsLineByLine(t, stderr, "stderr")

	return stdout.String(), stderr.String(), err
}

func RunTerragruntCommandWithOutput(t *testing.T, command string) (string, string, error) {
	t.Helper()

	return RunTerragruntCommandWithOutputWithContext(t, t.Context(), command)
}

func RunTerragruntRedirectOutput(t *testing.T, command string, writer io.Writer, errwriter io.Writer) {
	t.Helper()

	if err := RunTerragruntCommand(t, command, writer, errwriter); err != nil {
		stdout := "(see log output above)"
		if stdoutAsBuffer, stdoutIsBuffer := writer.(*bytes.Buffer); stdoutIsBuffer {
			stdout = stdoutAsBuffer.String()
		}

		stderr := "(see log output above)"
		if stderrAsBuffer, stderrIsBuffer := errwriter.(*bytes.Buffer); stderrIsBuffer {
			stderr = stderrAsBuffer.String()
		}

		t.Fatalf("Failed to run Terragrunt command '%s' due to error: %s\n\nStdout: %s\n\nStderr: %s", command, errors.ErrorStack(err), stdout, stderr)
	}
}

func CreateEmptyStateFile(t *testing.T, testPath string) {
	t.Helper()

	// create empty terraform.tfstate file
	file, err := os.Create(util.JoinPath(testPath, TerraformState))
	require.NoError(t, err)
	require.NoError(t, file.Close())
}

func RunTerragruntValidateInputs(t *testing.T, moduleDir string, extraArgs []string, isSuccessTest bool) {
	t.Helper()

	maybeNested := filepath.Join(moduleDir, "module")
	if util.FileExists(maybeNested) {
		// Nested module test case with included file, so run terragrunt from the nested module.
		moduleDir = maybeNested
	}

	cmd := fmt.Sprintf(
		"terragrunt hcl validate --inputs %s --log-level trace --non-interactive --working-dir %s",
		strings.Join(extraArgs, " "),
		moduleDir,
	)
	t.Logf("Command: %s", cmd)
	_, _, err := RunTerragruntCommandWithOutput(t, cmd)

	if isSuccessTest {
		require.NoError(t, err)
	} else {
		require.Error(t, err)
	}
}

func CreateTmpTerragruntConfigWithParentAndChild(t *testing.T, parentPath string, childRelPath string, s3BucketName string, parentConfigFileName string, childConfigFileName string) string {
	t.Helper()

	tmpDir := t.TempDir()

	childDestPath := util.JoinPath(tmpDir, childRelPath)

	if err := os.MkdirAll(childDestPath, allPermissions); err != nil {
		t.Fatalf("Failed to create temp dir %s due to error %v", childDestPath, err)
	}

	parentTerragruntSrcPath := util.JoinPath(parentPath, parentConfigFileName)
	parentTerragruntDestPath := util.JoinPath(tmpDir, parentConfigFileName)
	CopyTerragruntConfigAndFillPlaceholders(t, parentTerragruntSrcPath, parentTerragruntDestPath, s3BucketName, "not-used", "not-used")

	childTerragruntSrcPath := util.JoinPath(util.JoinPath(parentPath, childRelPath), childConfigFileName)
	childTerragruntDestPath := util.JoinPath(childDestPath, childConfigFileName)
	CopyTerragruntConfigAndFillPlaceholders(t, childTerragruntSrcPath, childTerragruntDestPath, s3BucketName, "not-used", "not-used")

	return childTerragruntDestPath
}

func IsTerragruntProviderCacheEnabled(t *testing.T) bool {
	t.Helper()

	for _, envName := range []string{"TERRAGRUNT_PROVIDER_CACHE", "TG_PROVIDER_CACHE"} {
		if val := os.Getenv(envName); val != "" {
			providerCache, err := strconv.ParseBool(val)
			require.NoError(t, err)

			if providerCache {
				return true
			}
		}
	}

	return false
}

// HCLFilesInDir returns a list of all HCL files in a directory.
func HCLFilesInDir(t *testing.T, dir string) []string {
	t.Helper()

	files := []string{}

	walkFn := func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if d.IsDir() {
			return nil
		}

		if strings.HasSuffix(path, ".hcl") {
			files = append(files, path)
		}

		return nil
	}

	err := filepath.WalkDir(dir, walkFn)
	require.NoError(t, err)

	return files
}

// CopyDir copies the contents of the directory at src to dst.
func CopyDir(t *testing.T, src, dst string) {
	t.Helper()

	// First, ensure the destination directory exists
	require.NoError(t, os.MkdirAll(dst, allPermissions))

	err := filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		require.NoError(t, err)

		relPath, err := filepath.Rel(src, path)
		require.NoError(t, err)

		dstPath := filepath.Join(dst, relPath)

		if d.IsDir() {
			// Get the source directory info to preserve permissions
			srcInfo, err := os.Stat(path)
			require.NoError(t, err)
			require.NoError(t, os.MkdirAll(dstPath, srcInfo.Mode()))
		} else {
			// Ensure parent directory exists
			parentDir := filepath.Dir(dstPath)
			require.NoError(t, os.MkdirAll(parentDir, allPermissions))
			CopyFile(t, path, dstPath)
		}

		return nil
	})

	require.NoError(t, err)
}

// CopyFile copies a single file from src to dst and preserves permissions.
func CopyFile(t *testing.T, src, dst string) {
	t.Helper()

	sourceFile, err := os.Open(src)
	require.NoError(t, err)

	defer sourceFile.Close()

	destFile, err := os.Create(dst)
	require.NoError(t, err)

	defer destFile.Close()

	_, err = io.Copy(destFile, sourceFile)
	require.NoError(t, err)

	sourceInfo, err := os.Stat(src)
	require.NoError(t, err)

	require.NoError(t, os.Chmod(dst, sourceInfo.Mode()))
}

// isAWSResourceNotFoundError checks if an error indicates that an AWS resource (S3 bucket, DynamoDB table, etc.) was not found
func isAWSResourceNotFoundError(err error) bool {
	var apiErr smithy.APIError

	return errors.As(err, &apiErr) && (apiErr.ErrorCode() == "NoSuchBucket" ||
		apiErr.ErrorCode() == "NotFound" ||
		apiErr.ErrorCode() == "ResourceNotFoundException")
}
