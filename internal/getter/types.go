package getter

import (
	gcs "github.com/hashicorp/go-getter/gcs/v2"
	s3 "github.com/hashicorp/go-getter/s3/v2"
	getter "github.com/hashicorp/go-getter/v2"
)

// Re-exports of hashicorp/go-getter/v2 types so callers can avoid importing
// go-getter directly.
type (
	Client       = getter.Client
	Request      = getter.Request
	Mode         = getter.Mode
	Getter       = getter.Getter
	Detector     = getter.Detector
	GetResult    = getter.GetResult
	Decompressor = getter.Decompressor
)

// Mode constants re-exported from go-getter/v2.
const (
	ModeInvalid = getter.ModeInvalid
	ModeAny     = getter.ModeAny
	ModeFile    = getter.ModeFile
	ModeDir     = getter.ModeDir
)

// Detector type aliases for the concrete detectors used by callers that
// build their own detector chain (e.g. internal/tf for normalizeSourceURL).
type (
	GitHubDetector    = getter.GitHubDetector
	GitDetector       = getter.GitDetector
	BitBucketDetector = getter.BitBucketDetector
	GitLabDetector    = getter.GitLabDetector
	FileDetector      = getter.FileDetector
	S3Detector        = s3.Detector
	GCSDetector       = gcs.Detector
)

// Concrete getter type aliases for callers that need to assert on the
// underlying getter type (typically tests pinning the protocol set). The
// git and s3 slots are intentionally absent: GitGetter and S3Getter are
// Terragrunt's own types (defined in git.go and s3.go), not the upstream
// go-getter/v2 getters.
type (
	HgGetter        = getter.HgGetter
	SmbClientGetter = getter.SmbClientGetter
	SmbMountGetter  = getter.SmbMountGetter
	FileGetter      = getter.FileGetter
	GCSGetter       = gcs.Getter
)
