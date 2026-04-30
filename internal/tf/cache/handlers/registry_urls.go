package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"

	"github.com/gruntwork-io/terragrunt/internal/errors"
)

const (
	// well-known address for discovery URLs
	wellKnownURL = ".well-known/terraform.json"
)

var (
	DefaultRegistryURLs = &RegistryURLs{
		ModulesV1:   "/v1/modules",
		ProvidersV1: "/v1/providers",
	}
)

type RegistryURLs struct {
	ModulesV1   string `json:"modules.v1"`
	ProvidersV1 string `json:"providers.v1"`
}

func (urls *RegistryURLs) String() string {
	if b, err := json.Marshal(urls); err == nil {
		return string(b)
	}

	return fmt.Sprintf("%v, %v", urls.ModulesV1, urls.ProvidersV1)
}

func DiscoveryURL(ctx context.Context, registryName string) (*RegistryURLs, error) {
	url := fmt.Sprintf("https://%s/%s", registryName, wellKnownURL)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, errors.New(err)
	}

	resp, err := (&http.Client{}).Do(req)
	if err != nil {
		return nil, errors.New(err)
	}
	defer resp.Body.Close() //nolint:errcheck

	switch resp.StatusCode {
	case http.StatusNotFound, http.StatusInternalServerError:
		return nil, errors.New(NotFoundWellKnownURLError{wellKnownURL})
	case http.StatusOK:
	default:
		return nil, fmt.Errorf("%s returned %s", url, resp.Status)
	}

	content, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, errors.New(err)
	}

	urls := new(RegistryURLs)
	if err := json.Unmarshal(content, urls); err != nil {
		return nil, errors.New(err)
	}

	return urls, nil
}

// IsOfflineError returns true if the given error indicates that the registry
// could not be reached, and default URLs should be used instead.
func IsOfflineError(err error) bool {
	if errors.As(err, &NotFoundWellKnownURLError{}) {
		return true
	}

	var urlErr *url.Error

	return errors.As(err, &urlErr)
}
