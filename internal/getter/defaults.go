package getter

import (
	"net/http"

	gcs "github.com/hashicorp/go-getter/gcs/v2"
	s3 "github.com/hashicorp/go-getter/s3/v2"
	getter "github.com/hashicorp/go-getter/v2"
)

// newHTTPGetter constructs an HttpGetter with Netrc enabled (matching
// Terragrunt's historic behavior under v1's UpdateGetters customization)
// and an optional set of extra headers. Pass nil for `extra` to get the
// default getter; pass a non-nil header set to inject auth (used by
// WithHTTPAuth and WithHTTPSAuth for GitHub release downloads).
//
// XTerraformGet is left enabled (the default) so X-Terraform-Get redirects
// continue to work.
func newHTTPGetter(extra http.Header) *getter.HttpGetter {
	return &getter.HttpGetter{Netrc: true, Header: extra}
}

// buildGetters realizes the option set into the ordered slice of Getters
// the client will iterate. The order matters for v2's first-match detection:
//
//  1. tfr (Terraform Registry): must precede git so tfr:// wins forced
//     detection.
//  2. CAS git wrapper: when CAS is enabled it intercepts git URLs ahead of
//     the bare GitGetter so plain `git::` sources route through CAS.
//  3. Optional caller-prepended getters (tests).
//  4. The default protocol set: git, hg, smb, http(s), s3, gcs, file.
//
// File goes last so it doesn't claim sources that other detectors recognize.
func buildGetters(b *builder) []Getter {
	var (
		out         []Getter
		fileGetter  Getter
		gitGetter   Getter
		httpGetter  Getter
		httpsGetter Getter
	)

	fileGetter = new(getter.FileGetter)
	if b.fileCopy != nil {
		fileGetter = b.fileCopy
	}

	gitGetter = NewGitGetter()

	httpGetter = newHTTPGetter(b.httpExtraHeader)
	httpsGetter = newHTTPGetter(b.httpsExtraHeader)

	if b.tfRegistry != nil {
		out = append(out, b.tfRegistry)
	}

	if b.casStore != nil {
		out = append(out, NewCASGetter(b.logger, b.casStore, b.casCloneOpts))
	}

	out = append(out, b.prepended...)

	out = append(out,
		gitGetter,
		new(getter.HgGetter),
		new(getter.SmbClientGetter),
		new(getter.SmbMountGetter),
		httpGetter,
		httpsGetter,
		new(s3.Getter),
		new(gcs.Getter),
		fileGetter,
	)

	return out
}
