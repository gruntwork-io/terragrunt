package vhttp

import (
	"errors"
	"net/http"
	"net/url"
)

// ErrNoNetwork is returned by clients built from [NewNoNetworkClient],
// wrapped in a *[net/url.Error] so callers that key off [net/url.Error]
// (e.g. registry offline-detection) see the same error shape they would
// from a real DNS/dial failure. Match it with [errors.Is].
var ErrNoNetwork = errors.New("vhttp: network access not permitted")

// NewNoNetworkClient returns a [Client] whose every request fails with an
// error wrapping [ErrNoNetwork] inside a *[net/url.Error]. It lets tests
// assert that a code path performs no outbound HTTP, the same way
// [github.com/gruntwork-io/terragrunt/internal/vfs.NoSymlinkFS] and
// [github.com/gruntwork-io/terragrunt/internal/vexec.NoLookPathExec] do
// for their respective subsystems.
func NewNoNetworkClient() Client {
	return &http.Client{Transport: noNetworkTransport{}}
}

type noNetworkTransport struct{}

func (noNetworkTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	method := http.MethodGet

	urlStr := ""

	if req != nil {
		method = req.Method
		if req.URL != nil {
			urlStr = req.URL.String()
		}
	}

	return nil, &url.Error{Op: method, URL: urlStr, Err: ErrNoNetwork}
}
