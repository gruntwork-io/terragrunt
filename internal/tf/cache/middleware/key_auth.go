package middleware

import (
	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

type Authorization struct {
	Token string
}

// Validator validates tokens.
//
// To enhance security, we use token-based authentication to connect to the cache server in order to prevent unauthorized connections from third-party applications.
// Currently, the cache server only supports `x-api-key` token, the value of which can be any text.
func (auth *Authorization) Validator(bearerToken string, ctx echo.Context) (bool, error) {
	if bearerToken != auth.Token {
		return false, errors.Errorf("Authorization: token either expired or inexistent")
	}

	return true, nil
}

// KeyAuth returns an KeyAuth middleware.
func KeyAuth(token string) echo.MiddlewareFunc {
	auth := Authorization{
		Token: token,
	}

	return middleware.KeyAuthWithConfig(middleware.KeyAuthConfig{
		Skipper:    middleware.DefaultSkipper,
		KeyLookup:  "header:" + echo.HeaderAuthorization,
		AuthScheme: "Bearer",
		Validator:  auth.Validator,
	})
}
