package handlers

import (
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"github.com/pkg/errors"
)

const AuthorizationApiKeyHeaderName = "x-api-key"

type Authorization struct {
	Token string
}

// To enhance security, we use token-based authentication to connect to the cache server in order to prevent unauthorized connections from third-party applications.
// Currently, the cache server only supports `x-api-key` token, the value of which can be any text.
func (auth *Authorization) Auth(bearerToken string, ctx echo.Context) (bool, error) {
	if bearerToken != auth.Token {
		return false, errors.Errorf("Authorization: token either expired or inexistent")
	}

	return true, nil
}

func (auth *Authorization) MiddlewareFunc() echo.MiddlewareFunc {
	return middleware.KeyAuthWithConfig(middleware.KeyAuthConfig{
		Skipper:    middleware.DefaultSkipper,
		KeyLookup:  "header:" + echo.HeaderAuthorization,
		AuthScheme: "Bearer",
		Validator:  auth.Auth,
	})
}
