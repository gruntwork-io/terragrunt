package helpers

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/gruntwork-io/terragrunt/internal/tf/cliconfig"
	svchost "github.com/hashicorp/terraform-svchost"
	"github.com/puzpuzpuz/xsync/v4"
)

// Client is an HTTP client.
type Client struct {
	*http.Client

	credsSource *cliconfig.CredentialsSource
	cache       *xsync.Map[string, []byte]
}

func NewClient(credsSource *cliconfig.CredentialsSource) *Client {
	return &Client{
		Client:      &http.Client{},
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

	resp, err := client.Client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close() //nolint:errcheck

	bodyBytes, err := DecodeResponse(resp)
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

// DecodeResponse reads an HTTP response body, returning an error for non-200 status codes.
func DecodeResponse(resp *http.Response) ([]byte, error) {
	if resp.StatusCode != http.StatusOK {
		if resp.StatusCode == http.StatusTooManyRequests {
			return nil, fmt.Errorf("received HTTP 429 Too Many Requests from %s (rate limited by registry)", resp.Request.URL)
		}

		return nil, fmt.Errorf("received HTTP %d %s from %s", resp.StatusCode, http.StatusText(resp.StatusCode), resp.Request.URL)
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
