package cli

import (
	"context"
	"encoding/json"
	goErrors "errors"
	"fmt"
	"io"
	"net/http"

	"github.com/gruntwork-io/go-commons/errors"
)

const (
	// well-known address for discovery URLs.
	wellKnownURL = ".well-known/terraform.json"
)

var (
	//nolint:gochecknoglobals
	// DefaultRegistryURLs is the default registry URLs.
	DefaultRegistryURLs = &RegistryURLs{
		ModulesV1:   "/v1/modules",
		ProvidersV1: "/v1/providers",
	}
)

// RegistryURLs represents the URLs for the registry.
type RegistryURLs struct {
	ModulesV1   string `json:"modules.v1"`
	ProvidersV1 string `json:"providers.v1"`
}

// String returns the JSON representation of the RegistryURLs.
func (urls *RegistryURLs) String() string {
	// TODO: handle error
	b, _ := json.Marshal(urls) //nolint:errchkjson

	return string(b)
}

var (
	// ErrDefaultDiscoveryURL is the default discovery URL error.
	ErrDefaultDiscoveryURL = goErrors.New("failed to get registry URLs")
)

// DiscoveryURL returns the registry URLs from the well-known URL.
func DiscoveryURL(ctx context.Context, registryName string) (*RegistryURLs, error) {
	url := fmt.Sprintf("https://%s/%s", registryName, wellKnownURL)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := (&http.Client{}).Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close() //nolint:errcheck

	switch resp.StatusCode {
	case http.StatusNotFound, http.StatusInternalServerError:
		return nil, NotFoundWellKnownURLError{wellKnownURL}
	case http.StatusOK:
	default:
		return nil, ErrDefaultDiscoveryURL
	}

	content, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, errors.WithStackTrace(err)
	}

	urls := new(RegistryURLs)
	if err := json.Unmarshal(content, urls); err != nil {
		return nil, errors.WithStackTrace(err)
	}

	return urls, nil
}

// NotFoundWellKnownURLError represents a not found well-known URL error.
type NotFoundWellKnownURLError struct {
	url string
}

// Error returns the error message.
func (err NotFoundWellKnownURLError) Error() string {
	return err.url + " not found"
}
