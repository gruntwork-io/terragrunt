package getter

import (
	"github.com/gruntwork-io/terragrunt/internal/cas"
	"github.com/gruntwork-io/terragrunt/internal/vhttp"
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
//
// The http, https, and tfr resolvers all probe over c. [CASGetter]
// callers normally go through [WithDefaultGenericDispatch], which
// supplies the venv's client.
func DefaultSourceResolvers(c vhttp.Client, opts ...GenericFetcherOption) map[string]SourceResolver {
	var cfg genericFetcherConfig
	for _, opt := range opts {
		opt(&cfg)
	}

	tfr := NewTFRResolver().WithHTTPClient(vhttp.WithTimeout(c, tfrResolverTimeout))
	if cfg.tfrLogger != nil {
		tfr.WithLogger(cfg.tfrLogger)
	}

	if cfg.tfrImpl != "" {
		tfr.WithTofuImplementation(cfg.tfrImpl)
	}

	probeClient := vhttp.WithTimeout(c, httpResolverTimeout)

	httpRes := NewHTTPResolver()
	httpRes.Client = probeClient

	httpsRes := NewHTTPSResolver()
	httpsRes.Client = probeClient

	return map[string]SourceResolver{
		SchemeHTTP:  httpRes,
		SchemeHTTPS: httpsRes,
		SchemeS3:    NewS3Resolver(),
		SchemeGCS:   NewGCSResolver(),
		SchemeHg:    NewHgResolver(),
		SchemeTFR:   tfr,
	}
}
