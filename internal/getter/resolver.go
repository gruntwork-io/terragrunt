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
//
// The tfr resolver is always registered. CASGetter only claims tfr:// URLs
// when the matching fetcher is registered (gated on [WithTFRConfig], since
// [RegistryGetter] requires a logger at construction), so an unused tfr
// resolver entry is harmless. Pass [WithTFRConfig] to align its logger and
// tofu implementation with the fetcher so the probe and the fetch resolve
// against the same registry host.
func DefaultSourceResolvers(opts ...GenericFetcherOption) map[string]SourceResolver {
	var cfg genericFetcherConfig
	for _, opt := range opts {
		opt(&cfg)
	}

	tfr := NewTFRResolver()
	if cfg.tfrLogger != nil {
		tfr.WithLogger(cfg.tfrLogger)
	}

	if cfg.tfrImpl != "" {
		tfr.WithTofuImplementation(cfg.tfrImpl)
	}

	return map[string]SourceResolver{
		SchemeHTTP:  NewHTTPResolver(),
		SchemeHTTPS: NewHTTPSResolver(),
		SchemeS3:    NewS3Resolver(),
		SchemeGCS:   NewGCSResolver(),
		SchemeHg:    NewHgResolver(),
		SchemeTFR:   tfr,
	}
}
