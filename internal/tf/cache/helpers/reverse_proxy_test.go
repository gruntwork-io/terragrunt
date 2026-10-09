package helpers_test

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/tf/cache/helpers"
	"github.com/gruntwork-io/terragrunt/internal/tf/cache/router"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/stretchr/testify/assert"
)

func TestReverseProxyNewRequestNilTransportPanics(t *testing.T) {
	t.Parallel()

	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", nil)
	w := router.NewResponseWriter(httptest.NewRecorder())

	proxy := &helpers.ReverseProxy{Logger: logger.CreateLogger()}
	target := &url.URL{Scheme: "https", Host: "example.test"}

	assert.Panics(t, func() {
		_ = proxy.NewRequest(w, req, target)
	})
}
