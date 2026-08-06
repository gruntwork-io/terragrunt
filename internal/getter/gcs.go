package getter

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"

	"cloud.google.com/go/storage"
	upstreamgcs "github.com/hashicorp/go-getter/gcs/v2"
	getter "github.com/hashicorp/go-getter/v2"
	"google.golang.org/api/iterator"

	"github.com/gruntwork-io/terragrunt/internal/venv"
)

// ErrGCSInvalidFetchURL is returned when a GCS URL does not carry both a
// bucket and an object.
var ErrGCSInvalidFetchURL = errors.New("not a valid GCS URL")

// gcsCanonicalParts is the segment count from splitting
// `/storage/<version>/<bucket>/<object>` on "/" with limit 5.
const gcsCanonicalParts = 5

// Compile-time proof both getters still satisfy the interface.
var (
	_ getter.Getter = (*GCSGetter)(nil)
	_ getter.Getter = (*S3Getter)(nil)
)

// GCSGetter is Terragrunt's gcs-protocol getter.
//
// Detection is delegated to the upstream go-getter/gcs/v2 Getter, which does
// no I/O. Fetching is not, because the upstream Getter calls storage.NewClient
// with no options, which would put every object download on a transport the
// venv never sees.
type GCSGetter struct {
	v             *venv.Venv
	detector      upstreamgcs.Getter
	modeScanLimit int
}

// NewGCSGetter returns a gcs-protocol getter.
func NewGCSGetter(v *venv.Venv) *GCSGetter {
	v.RequireFS()
	v.RequireHTTP()

	return &GCSGetter{v: v, modeScanLimit: DefaultModeScanLimit}
}

// WithModeScanLimit caps how many objects [GCSGetter.Mode] inspects before it
// gives up with [ErrModeScanLimit].
func (g *GCSGetter) WithModeScanLimit(limit int) *GCSGetter {
	g.modeScanLimit = limit
	return g
}

// Detect delegates to the upstream getter.
func (g *GCSGetter) Detect(req *getter.Request) (bool, error) {
	return g.detector.Detect(req)
}

// Mode reports whether the URL names a single object or a prefix. Anything
// matching the prefix that is not exactly the requested object makes it a
// directory.
func (g *GCSGetter) Mode(ctx context.Context, u *url.URL) (_ getter.Mode, retErr error) {
	bucket, object, err := ParseGCSFetchURL(u)
	if err != nil {
		return 0, err
	}

	client, err := g.client(ctx)
	if err != nil {
		return 0, err
	}

	defer closeOnSuccess(client, &retErr)

	objects := client.Bucket(bucket).Objects(ctx, &storage.Query{Prefix: object})

	mode, err := ScanMode(g.modeScanLimit,
		func(yield func(string, error) bool) {
			for {
				attrs, err := objects.Next()
				if errors.Is(err, iterator.Done) {
					return
				}

				if err != nil {
					yield("", err)
					return
				}

				if !yield(attrs.Name, nil) {
					return
				}
			}
		},
		func(name string) (getter.Mode, bool) {
			if strings.HasSuffix(name, "/") || name != object {
				return getter.ModeDir, true
			}

			return 0, false
		},
	)
	if err != nil {
		return 0, fmt.Errorf("%w: %q", err, object)
	}

	return mode, nil
}

// Get downloads every object under the URL's prefix into req.Dst, preserving
// each object's path relative to that prefix.
func (g *GCSGetter) Get(ctx context.Context, req *getter.Request) (retErr error) {
	bucket, object, err := ParseGCSFetchURL(req.URL())
	if err != nil {
		return err
	}

	client, err := g.client(ctx)
	if err != nil {
		return err
	}

	defer closeOnSuccess(client, &retErr)

	if err := ResetGetterDst(g.v.FS, req); err != nil {
		return err
	}

	objects := client.Bucket(bucket).Objects(ctx, &storage.Query{Prefix: object})

	for {
		attrs, err := objects.Next()
		if errors.Is(err, iterator.Done) {
			break
		}

		if err != nil {
			return err
		}

		// A name ending in "/" is a directory placeholder, not content.
		if strings.HasSuffix(attrs.Name, "/") {
			continue
		}

		body, err := client.Bucket(bucket).Object(attrs.Name).NewReader(ctx)
		if err != nil {
			return err
		}

		dst := ObjectDst(req.Dst, object, attrs.Name)
		if err := WriteGetterObject(g.v.FS, req, dst, body); err != nil {
			return err
		}
	}

	return nil
}

// GetFile downloads the single object the URL names into req.Dst.
func (g *GCSGetter) GetFile(ctx context.Context, req *getter.Request) (retErr error) {
	bucket, object, err := ParseGCSFetchURL(req.URL())
	if err != nil {
		return err
	}

	client, err := g.client(ctx)
	if err != nil {
		return err
	}

	defer closeOnSuccess(client, &retErr)

	body, err := client.Bucket(bucket).Object(object).NewReader(ctx)
	if err != nil {
		return err
	}

	return WriteGetterObject(g.v.FS, req, req.Dst, body)
}

// ParseGCSFetchURL extracts the bucket and object from the canonical
// `https://www.googleapis.com/storage/<version>/<bucket>/<object>` form the
// upstream detector produces.
func ParseGCSFetchURL(u *url.URL) (bucket, object string, err error) {
	if !strings.Contains(u.Host, "googleapis.com") {
		return "", "", fmt.Errorf("%w: %q", ErrGCSInvalidFetchURL, u.String())
	}

	parts := strings.SplitN(strings.TrimPrefix(u.Path, "/"), "/", gcsCanonicalParts-1)
	if len(parts) != gcsCanonicalParts-1 || parts[0] != "storage" || parts[3] == "" {
		return "", "", fmt.Errorf("%w: %q", ErrGCSInvalidFetchURL, u.String())
	}

	return parts[2], parts[3], nil
}

func (g *GCSGetter) client(ctx context.Context) (*storage.Client, error) {
	return newVenvGCSClient(ctx, g.v.HTTP)
}
