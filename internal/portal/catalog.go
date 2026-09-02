package portal

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/gruntwork-io/terragrunt/internal/retry"
	"github.com/gruntwork-io/terragrunt/internal/vhttp"
	"github.com/gruntwork-io/terragrunt/pkg/log"
)

const (
	// catalogPath is where the portal serves the org's selected repositories.
	// No response names it either, so it is assembled from the base URL.
	catalogPath = "/api/v1/catalog/repositories"

	// catalogAttempts bounds the tries a rate-limited fetch makes, so a portal
	// that goes on refusing ends the fetch while the user is still watching.
	catalogAttempts = 3

	// catalogBackoff is the wait before a second try when the portal named no
	// wait of its own, doubling on each further try.
	catalogBackoff = time.Second

	// catalogTimeout bounds the whole fetch, retries and waits included. The
	// client threaded down from the venv carries no timeout of its own.
	catalogTimeout = 30 * time.Second

	maxCatalogRetryAfter = 10 * time.Second
)

// Repository is one repository the org selected in the portal. The catalog
// carries a URL and nothing else.
type Repository struct {
	URL string
}

// catalogBody is the shape the portal serves the catalog in. Unknown fields are
// ignored, so the portal can add one without breaking a released CLI.
type catalogBody struct {
	Repositories []repositoryBody `json:"repositories"`
}

type repositoryBody struct {
	URL string `json:"url"`
}

// FetchCatalog returns the repositories the org behind token selected in the
// portal at baseURL. An org that selected none comes back as an empty list,
// which is an answer rather than a failure.
//
// A credential the portal refuses returns [ErrCredentialRejected], a portal
// with no catalog for this user returns [ErrNoHostedCatalog], and a portal that
// gave no answer returns [ErrPortalUnreachable].
//
// A rate limit that outlasted the retries returns a [*RateLimitedError], any
// other refusal returns an [Error] carrying the [ErrorCode] the portal named,
// and a response that cannot be read returns [ErrMalformedResponse].
func FetchCatalog(
	ctx context.Context,
	l log.Logger,
	c vhttp.Client,
	baseURL string,
	token Secret,
) ([]Repository, error) {
	endpoint, err := url.JoinPath(baseURL, catalogPath)
	if err != nil {
		return nil, fmt.Errorf("building the portal catalog URL from %q: %w", redactURL(baseURL), err)
	}

	fetchCtx, cancel := context.WithTimeout(ctx, catalogTimeout)
	defer cancel()

	c = withoutRedirects(c)

	var repositories []Repository

	err = retry.Do(fetchCtx, catalogAttempts,
		func(ctx context.Context) error {
			var fetchErr error

			repositories, fetchErr = requestCatalog(ctx, l, c, endpoint, token)

			return fetchErr
		},
		func(attempt int, err error) (time.Duration, bool) {
			var limited *RateLimitedError
			if !errors.As(err, &limited) {
				return 0, false
			}

			return retryDelay(limited, attempt), true
		},
	)
	if err != nil {
		return nil, fetchFailure(ctx, fetchCtx, err)
	}

	return repositories, nil
}

// fetchFailure tells a fetch that ran out of its own time from a caller that
// gave up waiting on it. Both arrive as a context error, so which context is
// done is what separates them.
func fetchFailure(ctx, fetchCtx context.Context, err error) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}

	if fetchCtx.Err() != nil {
		return fmt.Errorf("%w: it did not answer within %s", ErrPortalUnreachable, catalogTimeout)
	}

	return err
}

// retryDelay is how long a rate-limited fetch waits before trying again. The
// portal's advice wins when it gave any, capped at maxCatalogRetryAfter.
// Without it the wait doubles with each attempt.
func retryDelay(limited *RateLimitedError, attempt int) time.Duration {
	if limited.RetryAfter > 0 {
		return min(limited.RetryAfter, maxCatalogRetryAfter)
	}

	return catalogBackoff * time.Duration(1<<(attempt-1))
}

// requestCatalog makes one attempt at the catalog endpoint. The credential is
// revealed into the header being sent and is held nowhere else.
func requestCatalog(
	ctx context.Context,
	l log.Logger,
	c vhttp.Client,
	endpoint string,
	token Secret,
) ([]Repository, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("building the portal catalog request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+token.Reveal())
	req.Header.Set("Accept", jsonContentType)

	resp, err := c.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrPortalUnreachable, err)
	}

	body := io.LimitReader(resp.Body, maxResponseBytes)

	defer func() {
		if _, err := io.Copy(io.Discard, body); err != nil {
			l.Debugf("Error draining response body: %v", err)
		}

		if err := resp.Body.Close(); err != nil {
			l.Warnf("Error closing response body: %v", err)
		}
	}()

	if resp.StatusCode != http.StatusOK {
		return nil, catalogRefusal(resp, body)
	}

	return parseCatalog(body)
}

// catalogRefusal sorts a refusal into the class the caller acts on.
func catalogRefusal(resp *http.Response, body io.Reader) error {
	switch resp.StatusCode {
	case http.StatusUnauthorized:
		return ErrCredentialRejected
	case http.StatusNotFound:
		return ErrNoHostedCatalog
	case http.StatusTooManyRequests:
		return &RateLimitedError{RetryAfter: retryAfter(resp.Header)}
	}

	refusal := newError(resp, body)

	if refusal.Code == ErrorCodeFeatureNotEnabled {
		return ErrNoHostedCatalog
	}

	return refusal
}

// parseCatalog reads a catalog the portal reported as served. A repository the
// portal named no URL for fails the response.
func parseCatalog(r io.Reader) ([]Repository, error) {
	body := &bodyReader{r: r}

	var raw json.RawMessage
	if err := json.NewDecoder(body).Decode(&raw); err != nil {
		return nil, body.wrap(err)
	}

	if raw[0] != '{' {
		return nil, ErrResponseNotObject
	}

	var parsed catalogBody
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrMalformedResponse, err)
	}

	repositories := make([]Repository, 0, len(parsed.Repositories))

	for _, entry := range parsed.Repositories {
		if entry.URL == "" {
			return nil, &MissingFieldError{Field: "repositories[].url"}
		}

		repositories = append(repositories, Repository(entry))
	}

	return repositories, nil
}
