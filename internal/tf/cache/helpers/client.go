package helpers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/gruntwork-io/terragrunt/internal/tf/cliconfig"
	"github.com/gruntwork-io/terragrunt/internal/vhttp"
	svchost "github.com/hashicorp/terraform-svchost"
	"github.com/puzpuzpuz/xsync/v4"
)

// maxResponseBody bounds a registry response body length.
const maxResponseBody = 32 << 20

// Client is the cache server's outbound HTTP client. It wraps a
// [vhttp.Client] with registry credential injection and a per-URL response
// cache.
type Client struct {
	httpClient vhttp.Client

	credsSource *cliconfig.CredentialsSource
	cache       *xsync.Map[string, []byte]
}

// NewClient returns a [Client] that dispatches requests through c.
// Pass [vhttp.NewOSClient] in production or a [vhttp.NewMemClient] in tests.
func NewClient(c vhttp.Client, credsSource *cliconfig.CredentialsSource) *Client {
	return &Client{
		httpClient:  c,
		credsSource: credsSource,
		cache:       xsync.NewMap[string, []byte](),
	}
}

// Do sends an HTTP request and decodes an HTTP response to the given `value`.
func (client *Client) Do(ctx context.Context, method, reqURL string, value any) (err error) {
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

	defer func() {
		err = errors.Join(err, resp.Body.Close())
	}()

	bodyBytes, err := decodeResponse(resp)
	if err != nil {
		return err
	}

	client.cache.Store(reqURL, bodyBytes)

	return unmarshalBody(bodyBytes, value)
}

// ReadBody reads r into memory, refusing a body past limit with
// [ResponseTooLargeError] rather than decoding part of one.
func ReadBody(r io.Reader, hint, limit int64) ([]byte, error) {
	limited := io.LimitReader(r, limit+1)

	buf := &bytes.Buffer{}
	if hint > 0 && hint <= limit {
		buf.Grow(int(hint))
	}

	n, err := buf.ReadFrom(limited)
	if err != nil {
		return nil, err
	}

	if n > limit {
		return nil, ResponseTooLargeError{Limit: limit}
	}

	return buf.Bytes(), nil
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

	reader, err := ResponseReader(resp)
	if err != nil {
		return nil, err
	}

	body, readErr := ReadBody(reader, resp.ContentLength, maxResponseBody)
	if err := errors.Join(readErr, reader.Close()); err != nil {
		return nil, err
	}

	return body, nil
}
