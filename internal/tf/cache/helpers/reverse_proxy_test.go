package helpers_test

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/tf/cache/helpers"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
)

func TestReverseProxyNewRequestNilTransportPanics(t *testing.T) {
	t.Parallel()

	e := echo.New()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", nil)
	ctx := e.NewContext(req, httptest.NewRecorder())

	proxy := &helpers.ReverseProxy{Logger: logger.CreateLogger()}
	target := &url.URL{Scheme: "https", Host: "example.test"}

	assert.Panics(t, func() {
		err := proxy.NewRequest(ctx, target)
		t.Errorf("NewRequest returned %v instead of panicking", err)
	})
}
