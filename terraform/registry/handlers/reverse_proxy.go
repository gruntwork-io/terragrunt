package handlers

import (
	"net/http"
	"net/http/httputil"
	"net/url"

	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/labstack/echo/v4"
)

type ReverseProxy struct {
	ServerURL *url.URL

	Rewrite        func(*httputil.ProxyRequest)
	ModifyResponse func(resp *http.Response) error
	ErrorHandler   func(http.ResponseWriter, *http.Request, error)
}

func (reverseProxy ReverseProxy) WithRewrite(fn func(req *httputil.ProxyRequest)) *ReverseProxy {
	reverseProxy.Rewrite = fn
	return &reverseProxy
}

func (reverseProxy ReverseProxy) WithModifyResponse(fn func(resp *http.Response) error) *ReverseProxy {
	reverseProxy.ModifyResponse = fn
	return &reverseProxy
}

func (reverseProxy *ReverseProxy) NewRequest(ctx echo.Context, targetURL *url.URL) error {
	proxy := &httputil.ReverseProxy{
		Rewrite: func(req *httputil.ProxyRequest) {
			req.Out.Host = targetURL.Host
			req.Out.URL = targetURL

			if reverseProxy.Rewrite != nil {
				reverseProxy.Rewrite(req)
			}
		},
		ModifyResponse: func(resp *http.Response) error {
			if reverseProxy.ModifyResponse != nil {
				return reverseProxy.ModifyResponse(resp)
			}
			return nil
		},
		ErrorHandler: func(resp http.ResponseWriter, req *http.Request, err error) {
			log.Errorf("remote %s unreachable, could not forward: %v", targetURL, err)
			ctx.Error(echo.NewHTTPError(http.StatusServiceUnavailable))

			if reverseProxy.ErrorHandler != nil {
				reverseProxy.ErrorHandler(resp, req, err)
			}
		},
	}

	proxy.ServeHTTP(ctx.Response(), ctx.Request())
	return nil
}
