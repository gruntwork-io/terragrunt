package getter

import (
	"github.com/gruntwork-io/terragrunt/internal/cas"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
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
// resolver entry is harmless. Pass [WithDispatchLogger], [WithDispatchFS], and
// [WithTFRConfig] to align its logger and tofu implementation with the fetcher
// so the probe and the fetch resolve against the same registry host, and
// [WithDispatchEnv] so the probe carries the same registry credentials.
//
// The http, https, and tfr resolvers all probe over c, and the hg resolver
// spawns `hg` through v's executor. [CASGetter] callers normally go through
// [WithDefaultGenericDispatch], which supplies both.
func DefaultSourceResolvers(
	v *venv.Venv,
	c vhttp.Client,
	opts ...GenericFetcherOption,
) map[string]SourceResolver {
	// The probe client rather than v's own: a caller that overrode it expects
	// the probe to ride the override, and everything else still comes from v.
	rv := v.WithHTTP(c)

	var cfg genericFetcherConfig
	for _, opt := range opts {
		opt(&cfg)
	}

	tfr := NewTFRResolver().
		WithHTTPClient(vhttp.WithTimeout(c, tfrResolverTimeout)).
		WithAuth(RegistryAuth{Env: cfg.env, ReadUserConfig: vfs.IsOSFS(cfg.fs)})

	if cfg.tfrEnabled {
		requireLoggerFS(&cfg, SchemeTFR)
		tfr.WithLogger(cfg.logger)
	}

	if cfg.tfrImpl != "" {
		tfr.WithTofuImplementation(cfg.tfrImpl)
	}

	probeClient := vhttp.WithTimeout(c, httpResolverTimeout)

	httpRes := NewHTTPResolver()
	httpRes.Client = probeClient

	httpsRes := NewHTTPSResolver()
	httpsRes.Client = probeClient

	resolvers := map[string]SourceResolver{
		SchemeHTTP:  httpRes,
		SchemeHTTPS: httpsRes,
		SchemeS3:    NewS3Resolver(rv),
		SchemeGCS:   NewGCSResolver(rv),
		SchemeHg:    NewHgResolver(rv.Exec),
		SchemeTFR:   tfr,
	}

	if cfg.ociHolder != nil {
		requireLoggerFS(&cfg, SchemeOCI)
		resolvers[SchemeOCI] = NewOCIResolver(cfg.logger, cfg.ociHolder.store(cfg.logger))
	}

	return resolvers
}
