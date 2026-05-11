package getter

import (
	"github.com/gruntwork-io/terragrunt/internal/cas"
)

// SourceResolver is re-exported so callers configuring CASGetter only
// need to import internal/getter.
type SourceResolver = cas.SourceResolver

// DefaultSourceResolvers returns the per-scheme resolvers CASGetter dispatches
// through. SMB has no cheap probe so smb:// sources fall through to the
// no-resolver path in [cas.CAS.FetchSource] (download then content-hash); git
// is handled separately by [cas.CAS.Clone].
func DefaultSourceResolvers() map[string]SourceResolver {
	return map[string]SourceResolver{
		"http":  NewHTTPResolver(),
		"https": NewHTTPSResolver(),
		"s3":    NewS3Resolver(),
		"gcs":   NewGCSResolver(),
		"hg":    NewHgResolver(),
	}
}
