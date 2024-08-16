package cli

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
	// TODO: handle error
	b, _ := json.Marshal(urls) //nolint:errcheck,errchkjson
	return string(b)
}

func DiscoveryURL(ctx context.Context, registryName string) (*RegistryURLs, error) {
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
	case http.StatusNotFound, http.StatusInternalServerError:
		return nil, errors.WithStackTrace(NotFoundWellKnownURL{wellKnownURL})
	case http.StatusOK:
	default:
		return nil, fmt.Errorf("%s returned %s", url, resp.Status)
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

type NotFoundWellKnownURL struct {
	url string
}

func (err NotFoundWellKnownURL) Error() string {
	return err.url + " not found"
}
