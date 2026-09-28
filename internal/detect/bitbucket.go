// Package detect holds the go-getter source detectors Terragrunt owns rather
// than taking from go-getter itself.
package detect

import (
	"fmt"
	"net/url"
	"strings"
)

const bitBucketPrefix = "bitbucket.org/"

// BitBucket rewrites "bitbucket.org/owner/repo" shorthand into a URL the git
// getter understands.
//
// go-getter/v2's BitBucketDetector asks the BitBucket API whether a repository
// is Git or Mercurial before rewriting. BitBucket dropped Mercurial in July
// 2021, and that detector already turns every non-git answer into an error, so
// the request confirms the one answer it still accepts. It also goes through
// the package-level http.Get, which no test can redirect.
type BitBucket struct{}

// Detect implements go-getter's Detector.
func (d *BitBucket) Detect(src, _ string) (string, bool, error) {
	if !strings.HasPrefix(src, bitBucketPrefix) {
		return "", false, nil
	}

	u, err := url.Parse("https://" + src)
	if err != nil {
		return "", true, fmt.Errorf("error parsing BitBucket URL: %w", err)
	}

	if !strings.HasSuffix(u.Path, ".git") {
		u.Path += ".git"
	}

	return "git::" + u.String(), true, nil
}
