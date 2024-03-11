package handlers

import (
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"github.com/pkg/errors"
)

type Authorization struct {
	Token string
}

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
