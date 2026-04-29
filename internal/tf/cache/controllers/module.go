package controllers

import (
	"github.com/gruntwork-io/terragrunt/internal/tf/cache/handlers"
	"github.com/gruntwork-io/terragrunt/internal/tf/cache/router"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/labstack/echo/v4"
)

const (
	moduleName = "modules.v1"
	modulePath = "/modules"
)

// ModuleController exposes the modules.v1 registry protocol on the Terragrunt
// cache server. It accepts requests authenticated with the cache server's API key
// and forwards them to the upstream registry with the user's real credentials,
// fixing 403s on nested module lookups when the user-set TF_TOKEN_<host> is
// overridden by the cache server's token.
type ModuleController struct {
	*router.Router

	AuthMiddleware     echo.MiddlewareFunc
	ProxyModuleHandler *handlers.ProxyModuleHandler
	Logger             log.Logger
}

// Endpoints implements controllers.Endpointer.
func (c *ModuleController) Endpoints() map[string]any {
	return map[string]any{moduleName: c.URL().Path}
}

// Register implements router.Controller.
func (c *ModuleController) Register(r *router.Router) {
	c.Router = r.Group(modulePath)

	if c.AuthMiddleware != nil {
		c.Use(c.AuthMiddleware)
	}

	c.GET("/:cache_request_id/:registry_name/*", c.proxyAction)
}

func (c *ModuleController) proxyAction(ctx echo.Context) error {
	registryName := ctx.Param("registry_name")
	rest := ctx.Param("*")

	return c.ProxyModuleHandler.Proxy(ctx, registryName, rest)
}
