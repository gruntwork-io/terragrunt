package helpers

import (
	"context"
	"encoding/json"
	"io"
	"net/http"

	"github.com/gruntwork-io/terragrunt/internal/tf/cliconfig"
	"github.com/gruntwork-io/terragrunt/internal/vhttp"
	svchost "github.com/hashicorp/terraform-svchost"
	"github.com/puzpuzpuz/xsync/v4"
)

// Client is the cache server's outbound HTTP client. It wraps a
// [vhttp.Client] with registry credential injection and a per-URL response
// cache.
type Client struct {
	httpClient vhttp.Client

	credsSource *cliconfig.CredentialsSource
	cache       *xsync.Map[string, []byte]
}

// NewClient returns a [Client] that dispatches requests through httpClient.
// Pass [vhttp.NewOSClient] in production or a [vhttp.NewMemClient] in tests.
func NewClient(httpClient vhttp.Client, credsSource *cliconfig.CredentialsSource) *Client {
	return &Client{
		httpClient:  httpClient,
		credsSource: credsSource,
		cache:       xsync.NewMap[string, []byte](),
	}
}

// Do sends an HTTP request and decodes an HTTP response to the given `value`.
func (client *Client) Do(ctx context.Context, method, reqURL string, value any) error {
	if bodyBytes, ok := client.cache.Load(reqURL); ok {
		return unmarshalBody(bodyBytes, value)
	}

	req, err := http.NewRequestWithContext(ctx, method, reqURL, nil)
	if err != nil {
		return err
	}

	if client.credsSource != nil {
		hostname := svchost.Hostname(req.URL.Hostname())
		if creds := client.credsSource.ForHost(hostname); creds != nil {
			creds.PrepareRequest(req)
		}
	}

	resp, err := client.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close() //nolint:errcheck

	bodyBytes, err := decodeResponse(resp)
	if err != nil {
		return err
	}

	client.cache.Store(reqURL, bodyBytes)

	return unmarshalBody(bodyBytes, value)
}

func unmarshalBody(data []byte, value any) error {
	if data == nil {
		return nil
	}

	if err := json.Unmarshal(data, value); err != nil {
		return err
	}

	return nil
}

func decodeResponse(resp *http.Response) ([]byte, error) {
	if resp.StatusCode != http.StatusOK {
		return nil, nil
	}

	buffer, err := ResponseBuffer(resp)
	if err != nil {
		return nil, err
	}

	bodyBytes, err := io.ReadAll(buffer)
	if err != nil {
		return nil, err
	}

	resp.Body = io.NopCloser(buffer)

	return bodyBytes, nil
}
