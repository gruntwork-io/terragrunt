package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/gruntwork-io/go-commons/errors"
)

const (
	// well-known address for discovery URLs
	wellKnownURL = ".well-known/terraform.json"
)

var (
	DefaultDiscoveryURL = &ProviderDirectDiscoveryURL{
		ModulesV1:   "/v1/modules",
		ProvidersV1: "/v1/providers",
	}
)

type ProviderDirectDiscoveryURL struct {
	ModulesV1   string `json:"modules.v1"`
	ProvidersV1 string `json:"providers.v1"`
}

func (urls *ProviderDirectDiscoveryURL) String() string {
	b, _ := json.Marshal(urls) //nolint:errcheck
	return string(b)
}

func DiscoveryURL(ctx context.Context, registryName string) (*ProviderDirectDiscoveryURL, error) {
	url := fmt.Sprintf("https://%s/%s", registryName, wellKnownURL)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, errors.WithStackTrace(err)
	}

	resp, err := (&http.Client{}).Do(req)
	if err != nil {
		return nil, errors.WithStackTrace(err)
	}
	defer resp.Body.Close() //nolint:errcheck

	switch resp.StatusCode {
	case http.StatusNotFound:
		return nil, errors.WithStackTrace(NotFoundWellKnownURL{wellKnownURL})
	case http.StatusOK:
	default:
		return nil, fmt.Errorf("%s returned %s", url, resp.Status)
	}

	content, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, errors.WithStackTrace(err)
	}

	urls := new(ProviderDirectDiscoveryURL)
	if err := json.Unmarshal(content, urls); err != nil {
		return nil, errors.WithStackTrace(err)
	}
	return urls, nil
}
