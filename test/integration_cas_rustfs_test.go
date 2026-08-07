//go:build docker

package test_test

import (
	"bytes"
	"context"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/stretchr/testify/require"

	tgcas "github.com/gruntwork-io/terragrunt/internal/cas"
	tggetter "github.com/gruntwork-io/terragrunt/internal/getter"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/internal/vhttp"
	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"
)

// TestCAS_S3_RustFS_ProbeAvoidsRedownload verifies the S3 → CAS path
// against an in-Docker RustFS instance: a second CASGetter request for
// the same object skips the download when HeadObject reports the same
// version metadata.
func TestCAS_S3_RustFS_ProbeAvoidsRedownload(t *testing.T) { //nolint: paralleltest
	endpoint := setupRustFSForCAS(t)

	bucket := "cas-test-" + strings.ToLower(helpers.UniqueID())
	key := "modules/example.tar.gz"

	s3Client := newRustFSClient(t, endpoint)
	createRustFSBucket(t, s3Client, bucket)
	uploadRustFSObject(t, s3Client, bucket, key, makeModuleArchive(t))

	// CASGetter wired with the default generic dispatch (S3 fetcher
	// + S3 resolver). The S3 client's endpoint and credentials come
	// from the standard AWS_* env vars; RustFS speaks plain HTTP and
	// path-style URLs, so we set the resolver's NewClient hook to
	// match.
	storePath := filepath.Join(helpers.TmpDirWOSymlinks(t), "store")
	c, err := tgcas.New(venvtest.NewWithOSFS(), tgcas.WithStorePath(storePath))
	require.NoError(t, err)

	v := venv.OSVenv()

	resolvers := tggetter.DefaultSourceResolvers(v.HTTP, v.Exec)
	resolvers[tggetter.SchemeS3] = newRustFSS3Resolver(t, endpoint)

	g := tggetter.NewCASGetter(logger.CreateLogger(), c, v, &tgcas.CloneOptions{},
		tggetter.WithGenericFetchers(tggetter.DefaultGenericFetchers(v)),
		tggetter.WithGenericResolvers(resolvers),
	)

	client := &tggetter.Client{Getters: []tggetter.Getter{g}}

	// Embed credentials in the URL query so the bare go-getter
	// s3.Getter takes its endpoint-override branch. Without
	// aws_access_key_id/secret in the query it assumes amazonaws.com
	// and ignores the RustFS endpoint. The resolver's NewClient
	// hook configures BaseEndpoint separately for its HeadObject
	// probe.
	src := rustfsSourceURL(t, endpoint, bucket, key)

	first := filepath.Join(helpers.TmpDirWOSymlinks(t), "first")
	_, err = client.Get(t.Context(), &tggetter.Request{
		Src:     src,
		Dst:     first,
		GetMode: tggetter.ModeAny,
	})
	require.NoError(t, err)
	require.FileExists(t, filepath.Join(first, "main.tf"))

	second := filepath.Join(helpers.TmpDirWOSymlinks(t), "second")
	_, err = client.Get(t.Context(), &tggetter.Request{
		Src:     src,
		Dst:     second,
		GetMode: tggetter.ModeAny,
	})
	require.NoError(t, err)
	require.FileExists(t, filepath.Join(second, "main.tf"))
}

// setupRustFSForCAS spins up the same RustFS container the existing
// integration tests use and exports the AWS_* env vars the SDK config
// chain reads.
func setupRustFSForCAS(t *testing.T) string {
	t.Helper()

	_, addr := helpers.RunContainer(
		t,
		"rustfs/rustfs:1.0.0-beta.2@sha256:6bd08dc511cebe0a4b5c35c266db465c7eb92cf3df4321c69967be66fe4cb395",
		9000,
		testcontainers.WithCmd("/data"),
		testcontainers.WithWaitStrategy(wait.ForLog("Starting:")),
	)

	t.Setenv("AWS_ACCESS_KEY_ID", "rustfsadmin")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "rustfsadmin")
	t.Setenv("AWS_DEFAULT_REGION", "us-east-1")

	return addr
}

func newRustFSClient(t *testing.T, endpoint string) *s3.Client {
	t.Helper()

	c, err := newRustFSClientFor(t.Context(), endpoint)
	require.NoError(t, err)

	return c
}

// newRustFSClientFor builds an S3 client wired to the RustFS endpoint
// using ctx for credential / IMDS lookups. Used by the resolver's
// NewClient hook so the caller's context propagates into SDK config
// load, matching how a production resolver would honor request
// cancellation.
func newRustFSClientFor(ctx context.Context, endpoint string) (*s3.Client, error) {
	cfg, err := config.LoadDefaultConfig(
		ctx,
		config.WithRegion("us-east-1"),
		config.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider("rustfsadmin", "rustfsadmin", ""),
		),
	)
	if err != nil {
		return nil, err
	}

	return s3.NewFromConfig(cfg, func(o *s3.Options) {
		o.BaseEndpoint = aws.String(endpoint)
		o.UsePathStyle = true
	}), nil
}

// newRustFSS3Resolver returns an S3Resolver that talks to RustFS via
// the SDK's path-style endpoint, mirroring how a real S3Resolver would
// be configured for an S3-compatible object store.
func newRustFSS3Resolver(t *testing.T, endpoint string) *tggetter.S3Resolver {
	t.Helper()

	r := tggetter.NewS3Resolver(vhttp.NewNoNetworkClient())
	r.NewClient = func(ctx context.Context, _ string) (tggetter.S3API, error) {
		return newRustFSClientFor(ctx, endpoint)
	}

	return r
}

func createRustFSBucket(t *testing.T, c *s3.Client, bucket string) {
	t.Helper()

	_, err := c.CreateBucket(t.Context(), &s3.CreateBucketInput{
		Bucket: aws.String(bucket),
	})
	require.NoError(t, err)
}

func uploadRustFSObject(t *testing.T, c *s3.Client, bucket, key string, body []byte) {
	t.Helper()

	_, err := c.PutObject(t.Context(), &s3.PutObjectInput{
		Bucket:            aws.String(bucket),
		Key:               aws.String(key),
		Body:              bytes.NewReader(body),
		ChecksumAlgorithm: types.ChecksumAlgorithmSha256,
	})
	require.NoError(t, err)
}

// rustfsSourceURL builds the `s3::<scheme>://<host>/<bucket>/<key>`
// go-getter source string with RustFS credentials embedded in the
// query. The bare s3 getter parses the access-key query params and
// uses that as a signal to override its endpoint to u.Host, point at
// the testcontainer instead of real AWS.
func rustfsSourceURL(t *testing.T, endpoint, bucket, key string) string {
	t.Helper()

	u, err := url.Parse(endpoint)
	require.NoError(t, err)

	q := url.Values{}
	q.Set("aws_access_key_id", "rustfsadmin")
	q.Set("aws_access_key_secret", "rustfsadmin")

	out := url.URL{
		Scheme:   u.Scheme,
		Host:     u.Host,
		Path:     "/" + bucket + "/" + key,
		RawQuery: q.Encode(),
	}

	return "s3::" + out.String()
}

// TestS3PrefixDownloadReproducesLayout drives the s3 getter's directory path
// against RustFS: a source naming a key prefix rather than a single object
// must list every key below it and land each one at its position relative to
// that prefix, nesting included.
//
// The single-object test above resolves to ModeFile and never reaches
// S3Getter.Get, so this is what covers the paginated listing and the
// key-to-path mapping.
func TestS3PrefixDownloadReproducesLayout(t *testing.T) { //nolint: paralleltest
	endpoint := setupRustFSForCAS(t)

	bucket := "prefix-test-" + strings.ToLower(helpers.UniqueID())
	prefix := "modules/vpc"

	s3Client := newRustFSClient(t, endpoint)
	createRustFSBucket(t, s3Client, bucket)

	// A nested key must keep its subdirectory below the destination.
	want := map[string]string{
		"main.tf":       "# root",
		"variables.tf":  "# vars",
		"sub/nested.tf": "# nested",
	}

	for name, body := range want {
		uploadRustFSObject(t, s3Client, bucket, prefix+"/"+name, []byte(body))
	}

	// Shares the prefix's leading characters but not its path segment, so it
	// must stay out of the download.
	uploadRustFSObject(t, s3Client, bucket, prefix+"-sibling.tf", []byte("# excluded"))

	v := venv.OSVenv()
	dst := filepath.Join(helpers.TmpDirWOSymlinks(t), "module")

	_, err := tggetter.NewClient(v).Get(t.Context(), &tggetter.Request{
		Src:     rustfsSourceURL(t, endpoint, bucket, prefix),
		Dst:     dst,
		GetMode: tggetter.ModeAny,
	})
	require.NoError(t, err)

	for name, body := range want {
		got, readErr := os.ReadFile(filepath.Join(dst, filepath.FromSlash(name)))
		require.NoError(t, readErr, "expected %s below the prefix", name)
		require.Equal(t, body, string(got))
	}

	require.NoFileExists(t, filepath.Join(dst, "-sibling.tf"))
}
