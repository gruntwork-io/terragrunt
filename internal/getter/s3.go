package getter

import (
	"net/url"

	s3 "github.com/hashicorp/go-getter/s3/v2"
	getter "github.com/hashicorp/go-getter/v2"
)

// S3Getter is Terragrunt's s3-protocol getter. It wraps the upstream
// go-getter/s3/v2 Getter, whose URL parser only accepts legacy
// path-style hostnames (`s3.amazonaws.com`, `s3-<region>.amazonaws.com`),
// and canonicalizes the other AWS S3 endpoint forms — virtual-host style
// and modern path style — into that shape so they fetch instead of
// failing URL validation.
type S3Getter struct {
	s3.Getter
}

// Detect delegates to the upstream getter and, when the request is
// claimed, rewrites an AWS S3 URL to the path-style form the upstream
// Get/GetFile/Mode parser accepts. Detect is the only hook where a
// getter may rewrite the source: Client.Get re-parses req.Src into the
// request URL after detection, while later mutation would be ignored.
// Non-AWS (S3-compatible) hosts pass through untouched.
func (g *S3Getter) Detect(req *getter.Request) (bool, error) {
	ok, err := g.Getter.Detect(req)
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
