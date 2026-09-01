package helpers

import (
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

// maxPreallocBody bounds how much memory a response's Content-Length is trusted
// to reserve before any of the body has arrived. Registry metadata sits orders of
// magnitude below it, so a larger value buys nothing and costs a header's word.
const maxPreallocBody = 32 << 20

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

	reader, err := ResponseReader(resp)
	if err != nil {
		return nil, err
	}

	body, readErr := readBody(reader, resp.ContentLength)
	if err := errors.Join(readErr, reader.Close()); err != nil {
		return nil, err
	}

	return body, nil
}

// readBody reads r to completion. A positive size, which [ResponseReader] clears
// when it unwraps a gzip stream, is the body's exact length, so the read needs one
// allocation rather than [io.ReadAll]'s repeated doubling. Anything above
// maxPreallocBody falls back to [io.ReadAll], so a registry advertising a bogus
// Content-Length cannot make Terragrunt reserve that memory up front.
func readBody(r io.Reader, size int64) ([]byte, error) {
	if size <= 0 || size > maxPreallocBody {
		return io.ReadAll(r)
	}

	body := make([]byte, size)
	if _, err := io.ReadFull(r, body); err != nil {
		return nil, err
	}

	return body, nil
}
