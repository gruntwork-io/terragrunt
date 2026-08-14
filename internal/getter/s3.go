package getter

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"net/url"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	upstreams3 "github.com/hashicorp/go-getter/s3/v2"
	getter "github.com/hashicorp/go-getter/v2"

	"github.com/gruntwork-io/terragrunt/internal/venv"
)

// ErrS3InvalidFetchURL is returned when an s3:// URL does not carry both a
// bucket and a key once Detect has canonicalized it to path style.
var ErrS3InvalidFetchURL = errors.New("not a valid S3 URL")

// s3DefaultRegion matches the region the upstream getter assumes when the URL
// names none, so an S3-compatible service that ignores the region keeps working.
const s3DefaultRegion = "us-east-1"

// s3PathParts is the segment count from splitting `/bucket/key` on "/" with
// limit 3: ["", "bucket", "key"].
const s3PathParts = 3

// s3AWSHostParts is the label count of a path-style AWS S3 host, which Detect
// has already canonicalized to `s3[-<region>].amazonaws.com`.
const s3AWSHostParts = 3

// S3Getter is Terragrunt's s3-protocol getter.
//
// Detection is delegated to the upstream go-getter/s3/v2 Getter, which does no
// I/O. Its URL parser only accepts legacy path-style hostnames
// (`s3.amazonaws.com`, `s3-<region>.amazonaws.com`), so [S3Getter.Detect]
// canonicalizes the other AWS endpoint forms into that shape. Fetching is not
// delegated, because the upstream Getter builds its own AWS session, which
// would put every object download on a transport the venv never sees.
type S3Getter struct {
	v             *venv.Venv
	detector      upstreams3.Getter
	modeScanLimit int
}

// NewS3Getter returns an s3-protocol getter.
func NewS3Getter(v *venv.Venv) *S3Getter {
	v.RequireFS()
	v.RequireHTTP()

	return &S3Getter{v: v, modeScanLimit: DefaultModeScanLimit}
}

// WithModeScanLimit caps how many keys [S3Getter.Mode] inspects before it
// gives up with [ErrModeScanLimit].
func (g *S3Getter) WithModeScanLimit(limit int) *S3Getter {
	g.modeScanLimit = limit
	return g
}

// Detect delegates to the upstream getter and, when the request is
// claimed, rewrites an AWS S3 URL to the path-style form the fetch
// methods below parse. Detect is the only hook where a getter may rewrite
// the source: Client.Get re-parses req.Src into the request URL after
// detection, while later mutation would be ignored. Non-AWS
// (S3-compatible) hosts pass through untouched.
func (g *S3Getter) Detect(req *getter.Request) (bool, error) {
	ok, err := g.detector.Detect(req)
	if err != nil || !ok {
		return ok, err
	}

	if u, perr := url.Parse(req.Src); perr == nil {
		if canonical, cok := canonicalAWSS3HTTPSURL(u); cok {
			req.Src = canonical
		}
	}

	return true, nil
}

// Mode reports whether the URL names a single object or a prefix, deciding
// each listed key with [ObjectMode]. A prefix matching nothing reports file
// mode so the download surfaces S3's own not-found error rather than a
// synthesized one.
func (g *S3Getter) Mode(ctx context.Context, u *url.URL) (getter.Mode, error) {
	target, err := ParseS3FetchURL(u)
	if err != nil {
		return 0, err
	}

	client, err := g.client(ctx, &target)
	if err != nil {
		return 0, err
	}

	// Without MaxKeys a scan limit above the page size ends at the page
	// boundary, and ScanMode reads the truncated listing as a file.
	out, err := client.ListObjectsV2(ctx, &s3.ListObjectsV2Input{
		Bucket:  aws.String(target.Bucket),
		Prefix:  aws.String(target.Key),
		MaxKeys: aws.Int32(int32(min(g.modeScanLimit, math.MaxInt32))),
	})
	if err != nil {
		return 0, err
	}

	mode, err := ScanMode(g.modeScanLimit,
		func(yield func(string, error) bool) {
			for _, obj := range out.Contents {
				if !yield(aws.ToString(obj.Key), nil) {
					return
				}
			}
		},
		func(key string) (getter.Mode, bool) {
			return ObjectMode(key, target.Key)
		},
	)
	if err != nil {
		return 0, fmt.Errorf("%w: %q", err, target.Key)
	}

	return mode, nil
}

// Get downloads every object under the URL's key prefix into req.Dst,
// preserving each object's path relative to that prefix.
func (g *S3Getter) Get(ctx context.Context, req *getter.Request) error {
	target, err := ParseS3FetchURL(req.URL())
	if err != nil {
		return err
	}

	client, err := g.client(ctx, &target)
	if err != nil {
		return err
	}

	if err := ResetGetterDst(g.v.FS, req); err != nil {
		return err
	}

	prefix := ListPrefix(target.Key)

	pages := s3.NewListObjectsV2Paginator(client, &s3.ListObjectsV2Input{
		Bucket: aws.String(target.Bucket),
		Prefix: aws.String(prefix),
	})

	for pages.HasMorePages() {
		page, err := pages.NextPage(ctx)
		if err != nil {
			return err
		}

		for _, obj := range page.Contents {
			key := aws.ToString(obj.Key)

			// A key ending in "/" is a directory placeholder, not content.
			if strings.HasSuffix(key, "/") {
				continue
			}

			dst, err := ObjectDst(req.Dst, prefix, key)
			if err != nil {
				return err
			}

			body, err := g.object(ctx, client, &target, key, "")
			if err != nil {
				return err
			}

			if err := WriteGetterObject(g.v.FS, req, dst, body); err != nil {
				return err
			}
		}
	}

	return nil
}

// GetFile downloads the single object the URL names into req.Dst.
func (g *S3Getter) GetFile(ctx context.Context, req *getter.Request) error {
	target, err := ParseS3FetchURL(req.URL())
	if err != nil {
		return err
	}

	client, err := g.client(ctx, &target)
	if err != nil {
		return err
	}

	body, err := g.object(ctx, client, &target, target.Key, target.Version)
	if err != nil {
		return err
	}

	return WriteGetterObject(g.v.FS, req, req.Dst, body)
}

// S3FetchTarget is a path-style s3:// URL resolved into what the SDK needs.
type S3FetchTarget struct {
	Creds    aws.CredentialsProvider
	Region   string
	Bucket   string
	Key      string
	Version  string
	Profile  string
	Endpoint string
}

// redactedURL renders u for an error message with its query dropped. An S3
// URL carries credentials there, and a rejected URL still reaches logs.
func redactedURL(u *url.URL) string {
	clean := *u
	clean.RawQuery = ""

	return clean.String()
}

// ParseS3FetchURL resolves a path-style S3 URL. Detect has already rewritten
// the AWS virtual-host and modern path-style forms, so only
// `<host>/<bucket>/<key>` reaches here.
//
// Credentials supplied in the query are also the signal that the URL names an
// S3-compatible service rather than AWS, so they pin the endpoint to the URL's
// own host in path style. That mirrors the upstream getter, which callers rely
// on to reach non-AWS object stores.
func ParseS3FetchURL(u *url.URL) (S3FetchTarget, error) {
	pathParts := strings.SplitN(u.Path, "/", s3PathParts)
	if len(pathParts) != s3PathParts || pathParts[1] == "" || pathParts[2] == "" {
		return S3FetchTarget{}, fmt.Errorf("%w: %q", ErrS3InvalidFetchURL, redactedURL(u))
	}

	q := u.Query()

	target := S3FetchTarget{
		Bucket:  pathParts[1],
		Key:     pathParts[2],
		Version: q.Get("version"),
		Profile: q.Get("aws_profile"),
	}

	region, err := S3Region(u)
	if err != nil {
		return S3FetchTarget{}, err
	}

	target.Region = region

	keyID := q.Get("aws_access_key_id")
	secret := q.Get("aws_access_key_secret")
	token := q.Get("aws_access_token")

	if cmp.Or(keyID, secret, token) != "" {
		target.Creds = credentials.NewStaticCredentialsProvider(keyID, secret, token)
		target.Endpoint = u.Scheme + "://" + u.Host
	}

	return target, nil
}

// S3Region resolves the region for u. An AWS host encodes it in the first
// label; any other host belongs to an S3-compatible service, which carries it
// in the query instead.
func S3Region(u *url.URL) (string, error) {
	if !strings.Contains(u.Host, "amazonaws.com") {
		return cmp.Or(u.Query().Get("region"), s3DefaultRegion), nil
	}

	hostParts := strings.Split(u.Host, ".")
	if len(hostParts) != s3AWSHostParts {
		return "", fmt.Errorf("%w: %q", ErrS3InvalidFetchURL, redactedURL(u))
	}

	region, ok := S3RegionFromHostLabel(hostParts[0])
	if !ok {
		return "", fmt.Errorf("%w: %q", ErrS3InvalidFetchURL, redactedURL(u))
	}

	return region, nil
}

// object opens the object body. The caller closes it via writeGetterObject.
func (g *S3Getter) object(
	ctx context.Context,
	client *s3.Client,
	target *S3FetchTarget,
	key, version string,
) (io.ReadCloser, error) {
	in := &s3.GetObjectInput{
		Bucket: aws.String(target.Bucket),
		Key:    aws.String(key),
	}
	if version != "" {
		in.VersionId = aws.String(version)
	}

	out, err := client.GetObject(ctx, in)
	if err != nil {
		return nil, err
	}

	return out.Body, nil
}

func (g *S3Getter) client(ctx context.Context, target *S3FetchTarget) (*s3.Client, error) {
	opts := []func(*awsconfig.LoadOptions) error{
		awsconfig.WithRegion(target.Region),
		awsconfig.WithHTTPClient(g.v.HTTP),
	}

	if target.Profile != "" {
		opts = append(opts, awsconfig.WithSharedConfigProfile(target.Profile))
	}

	if target.Creds != nil {
		opts = append(opts, awsconfig.WithCredentialsProvider(target.Creds))
	}

	//nolint:forbidigo // WithHTTPClient above carries the venv's client, which is what the rule protects.
	cfg, err := awsconfig.LoadDefaultConfig(ctx, opts...)
	if err != nil {
		return nil, err
	}

	return s3.NewFromConfig(cfg, func(o *s3.Options) {
		if target.Endpoint != "" {
			o.BaseEndpoint = aws.String(target.Endpoint)
			o.UsePathStyle = true
		}
	}), nil
}
