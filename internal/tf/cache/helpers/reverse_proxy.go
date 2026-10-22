package helpers

import (
	"net/http"
	"net/http/httputil"
	"net/url"

	"github.com/gruntwork-io/terragrunt/internal/tf/cache/router"
	"github.com/gruntwork-io/terragrunt/internal/tf/cliconfig"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	svchost "github.com/hashicorp/terraform-svchost"
)

type ReverseProxy struct {
	ServerURL   *url.URL
	CredsSource *cliconfig.CredentialsSource

	// Transport carries the injected vhttp client's RoundTripper so the
	// proxied data path honors the same virtualization and pooling as the
	// discovery requests. Required: NewRequest panics on nil rather than
	// letting httputil.ReverseProxy silently fall back to
	// http.DefaultTransport and reach the real network un-injected.
	Transport http.RoundTripper

	Rewrite        func(*httputil.ProxyRequest)
	ModifyResponse func(resp *http.Response) error
	ErrorHandler   func(http.ResponseWriter, *http.Request, error)

	Logger log.Logger
}

func (rp ReverseProxy) WithModifyResponse(
	fn func(resp *http.Response) error,
) *ReverseProxy {
	rp.ModifyResponse = fn
	return &rp
}

// NewRequest forwards r to targetURL and streams the answer to w. A target
// that cannot be reached is answered with a 503.
//
// Panics when rp.Transport is nil.
func (rp *ReverseProxy) NewRequest(w router.ResponseWriter, r *http.Request, targetURL *url.URL) error {
	if rp.Transport == nil {
		panic(
			"helpers.ReverseProxy: nil Transport; wire the vhttp client's transport at construction",
		)
	}

	proxy := &httputil.ReverseProxy{
		Transport: rp.Transport,
		Rewrite: func(req *httputil.ProxyRequest) {
			req.Out.Host = targetURL.Host
			req.Out.URL = targetURL

			if rp.CredsSource != nil {
				hostname := svchost.Hostname(req.Out.URL.Hostname())
				if creds := rp.CredsSource.ForHost(hostname); creds != nil {
					creds.PrepareRequest(req.Out)
				}
			}

			if rp.Rewrite != nil {
				rp.Rewrite(req)
			}
		},
		ModifyResponse: func(resp *http.Response) error {
			if rp.ModifyResponse != nil {
				return rp.ModifyResponse(resp)
			}

			return nil
		},
		ErrorHandler: func(resp http.ResponseWriter, req *http.Request, err error) {
			rp.Logger.Errorf(
				"remote %s unreachable, could not forward: %v",
				targetURL,
				err,
			)
			router.WriteError(w, router.NewHTTPError(http.StatusServiceUnavailable))

			if rp.ErrorHandler != nil {
				rp.ErrorHandler(resp, req, err)
			}
		},
	}

	proxy.ServeHTTP(w, r)

	return nil
}
