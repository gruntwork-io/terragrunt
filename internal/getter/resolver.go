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
// Every resolver rides v: the http, https, and tfr probes go over its client
// and the hg resolver spawns `hg` through its executor. A caller overriding the
// probe client passes a venv carrying it ([venv.Venv.WithHTTP]), which is what
// [WithDefaultGenericDispatch] does.
func DefaultSourceResolvers(
	v *venv.Venv,
	opts ...GenericFetcherOption,
) map[string]SourceResolver {
	v.RequireExec()
	v.RequireHTTP()

	var cfg genericFetcherConfig
	for _, opt := range opts {
		opt(&cfg)
	}

	tfr := NewTFRResolver().
		WithHTTPClient(vhttp.WithTimeout(v.HTTP, tfrResolverTimeout)).
		WithAuth(RegistryAuth{Env: cfg.env, ReadUserConfig: vfs.IsOSFS(cfg.fs)})

	if cfg.tfrEnabled {
		requireLoggerFS(&cfg, SchemeTFR)
		tfr.WithLogger(cfg.logger)
	}

	if cfg.tfrImpl != "" {
		tfr.WithTofuImplementation(cfg.tfrImpl)
	}

	probeClient := vhttp.WithTimeout(v.HTTP, httpResolverTimeout)

	httpRes := NewHTTPResolver()
	httpRes.Client = probeClient

	httpsRes := NewHTTPSResolver()
	httpsRes.Client = probeClient

	resolvers := map[string]SourceResolver{
		SchemeHTTP:  httpRes,
		SchemeHTTPS: httpsRes,
		SchemeS3:    NewS3Resolver(v),
		SchemeGCS:   NewGCSResolver(v),
		SchemeHg:    NewHgResolver(v.Exec),
		SchemeTFR:   tfr,
	}

	if cfg.ociHolder != nil {
		requireLoggerFS(&cfg, SchemeOCI)
		resolvers[SchemeOCI] = NewOCIResolver(cfg.logger, cfg.ociHolder.store(cfg.logger))
	}

	return resolvers
}
