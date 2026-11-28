// OIDC federated-credential assertion sources.

package azurehelper

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"net/http"
	"net/url"
	"strings"

	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/pkg/log"
)

// oidcExchangeAudience is the audience Microsoft Entra expects for a
// federated credential assertion.
const oidcExchangeAudience = "api://AzureADTokenExchange"

// adoOIDCAPIVersion is the Azure DevOps REST API version of the OIDC token endpoint.
const adoOIDCAPIVersion = "7.1"

// maxOIDCTokenResponseBytes bounds the token-endpoint response so a
// misconfigured URL cannot stream an unbounded body into memory.
const maxOIDCTokenResponseBytes = 1 << 20

// assertionProvider returns a federated credential assertion (a JWT) for
// Microsoft Entra to exchange for an access token.
type assertionProvider func(ctx context.Context) (string, error)

// oidcTokenRequest describes one request for a federated token.
type oidcTokenRequest struct {
	query  url.Values
	header http.Header
	method string
	url    string
	token  string
	field  string
}

// oidcAssertionSource picks a service connection, a token, then a request url; nil selects the token file.
func oidcAssertionSource(l log.Logger, v *venv.Venv, cfg *AzureSessionConfig) assertionProvider {
	if request := adoTokenRequest(v.Env, cfg.ADOServiceConnectionID); request != nil {
		l.Debugf("azurehelper: OIDC assertion source is an Azure DevOps service connection")

		return func(ctx context.Context) (string, error) {
			return sendOIDCTokenRequest(ctx, v, request)
		}
	}

	if cfg.OIDCToken != "" {
		l.Debugf("azurehelper: OIDC assertion source is a token")

		return tokenAssertion(v, cfg.OIDCToken, cfg.OIDCTokenFilePath)
	}

	if cfg.OIDCTokenFilePath != "" {
		return nil
	}

	return oidcAssertionProvider(v)
}

// oidcAssertionProvider picks the request url source: explicit, GitHub Actions, then Azure DevOps; nil for none.
func oidcAssertionProvider(v *venv.Venv) assertionProvider {
	// GitHub Actions injects both variables into every job that requests the
	// id-token permission.
	if requestURL := firstEnv(v.Env, "ARM_OIDC_REQUEST_URL", "ACTIONS_ID_TOKEN_REQUEST_URL"); requestURL != "" {
		token := firstEnv(v.Env, "ARM_OIDC_REQUEST_TOKEN", "ACTIONS_ID_TOKEN_REQUEST_TOKEN")

		return func(ctx context.Context) (string, error) {
			return fetchOIDCAssertion(ctx, v, requestURL, token, "value")
		}
	}

	// Azure DevOps workload identity federation.
	if requestURL := firstEnv(v.Env, "SYSTEM_OIDCREQUESTURI"); requestURL != "" {
		token := firstEnv(v.Env, "SYSTEM_ACCESSTOKEN")

		return func(ctx context.Context) (string, error) {
			return fetchOIDCAssertion(ctx, v, requestURL, token, "oidcToken")
		}
	}

	return nil
}

// fetchOIDCAssertion requests a federated token from requestURL, authorizing
// with bearerToken, and reads the assertion out of the named JSON field.
func fetchOIDCAssertion(
	ctx context.Context,
	v *venv.Venv,
	requestURL, bearerToken, field string,
) (string, error) {
	return sendOIDCTokenRequest(ctx, v, &oidcTokenRequest{
		query:  url.Values{"audience": {oidcExchangeAudience}},
		method: http.MethodGet,
		url:    requestURL,
		token:  bearerToken,
		field:  field,
	})
}

// adoTokenRequest returns the Azure DevOps service connection request, or nil when an input is missing.
func adoTokenRequest(env map[string]string, serviceConnectionID string) *oidcTokenRequest {
	requestURL := firstEnv(env, "SYSTEM_OIDCREQUESTURI")
	token := firstEnv(env, "ARM_OIDC_REQUEST_TOKEN", "ACTIONS_ID_TOKEN_REQUEST_TOKEN", "SYSTEM_ACCESSTOKEN")

	if serviceConnectionID == "" || requestURL == "" || token == "" {
		return nil
	}

	header := http.Header{}
	// Azure DevOps answers 401 in place of a sign-in redirect when this header is set.
	header.Set("X-Tfs-Fedauthredirect", "Suppress")

	return &oidcTokenRequest{
		query: url.Values{
			"api-version":         {adoOIDCAPIVersion},
			"serviceConnectionId": {serviceConnectionID},
		},
		header: header,
		method: http.MethodPost,
		url:    requestURL,
		token:  token,
		field:  "oidcToken",
	}
}

// tokenAssertion returns token, which must equal the contents of tokenFile when the file is not empty.
func tokenAssertion(v *venv.Venv, token, tokenFile string) assertionProvider {
	v.RequireFS()

	return func(context.Context) (string, error) {
		if tokenFile == "" {
			return token, nil
		}

		contents, err := vfs.ReadFile(v.FS, tokenFile)
		if err != nil {
			return "", fmt.Errorf("reading OIDC token file %s: %w", tokenFile, err)
		}

		if len(contents) != 0 && string(contents) != token {
			return "", &OIDCTokenMismatchError{TokenFilePath: tokenFile}
		}

		return token, nil
	}
}

// sendOIDCTokenRequest sends r and returns the assertion from the JSON field that r names.
func sendOIDCTokenRequest(ctx context.Context, v *venv.Venv, r *oidcTokenRequest) (_ string, err error) {
	if r.token == "" {
		return "", &OIDCRequestTokenMissingError{RequestURL: r.url}
	}

	v.RequireHTTP()

	parsed, err := url.Parse(r.url)
	if err != nil {
		return "", fmt.Errorf("parsing OIDC request url: %w", err)
	}

	query := parsed.Query()
	maps.Copy(query, r.query)
	parsed.RawQuery = query.Encode()

	req, err := http.NewRequestWithContext(ctx, r.method, parsed.String(), nil)
	if err != nil {
		return "", fmt.Errorf("building OIDC token request: %w", err)
	}

	maps.Copy(req.Header, r.header)
	req.Header.Set("Authorization", "Bearer "+r.token)
	req.Header.Set("Accept", "application/json")

	resp, err := v.HTTP.Do(req)
	if err != nil {
		return "", fmt.Errorf("requesting OIDC token: %w", err)
	}

	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			err = errors.Join(err, fmt.Errorf("closing OIDC token response: %w", closeErr))
		}
	}()

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxOIDCTokenResponseBytes))
	if err != nil {
		return "", fmt.Errorf("reading OIDC token response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", &OIDCTokenRequestFailedError{
			StatusCode: resp.StatusCode,
			Body:       strings.TrimSpace(string(body)),
		}
	}

	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return "", fmt.Errorf("decoding OIDC token response: %w", err)
	}

	assertion, _ := payload[r.field].(string)
	if assertion == "" {
		return "", &OIDCTokenFieldMissingError{Field: r.field}
	}

	return assertion, nil
}
