package helpers

import (
	"context"
	"encoding/json"
	"io"
	"net/http"

	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/tf/cliconfig"
	svchost "github.com/hashicorp/terraform-svchost"
	"github.com/puzpuzpuz/xsync/v3"
)

// Client is an HTTP client.
type Client struct {
	*http.Client

	credsSource *cliconfig.CredentialsSource
	cache       *xsync.MapOf[string, []byte]
}

func NewClient(credsSource *cliconfig.CredentialsSource) *Client {
	return &Client{
		Client:      &http.Client{},
		credsSource: credsSource,
		cache:       xsync.NewMapOf[string, []byte](),
	}
}

// Do sends an HTTP request and decodes an HTTP response to the given `value`.
func (client *Client) Do(ctx context.Context, method, reqURL string, value any) error {
	if bodyBytes, ok := client.cache.Load(reqURL); ok {
		return unmarshalBody(bodyBytes, value)
	}

	req, err := http.NewRequestWithContext(ctx, method, reqURL, nil)
	if err != nil {
		return errors.New(err)
	}

	if client.credsSource != nil {
		hostname := svchost.Hostname(req.URL.Hostname())
		if creds := client.credsSource.ForHost(hostname); creds != nil {
			creds.PrepareRequest(req)
		}
	}

	resp, err := client.Client.Do(req)
	if err != nil {
		return errors.New(err)
	}
	defer resp.Body.Close() //nolint:errcheck

	bodyBytes, err := decodeResponse(resp)
	if err != nil {
		return errors.New(err)
	}

	client.cache.Store(reqURL, bodyBytes)

	return unmarshalBody(bodyBytes, value)
}

func unmarshalBody(data []byte, value any) error {
	if data == nil {
		return nil
	}

	if err := json.Unmarshal(data, value); err != nil {
		return errors.New(err)
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
		return nil, errors.New(err)
	}

	resp.Body = io.NopCloser(buffer)

	return bodyBytes, nil
}
