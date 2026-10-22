package controllers

import (
	"net/http"

	"github.com/gruntwork-io/terragrunt/internal/tf/cache/handlers"
	"github.com/gruntwork-io/terragrunt/internal/tf/cache/router"
	"github.com/gruntwork-io/terragrunt/pkg/log"
)

const (
	moduleName = "modules.v1"
	modulePath = "/modules"
)

// ModuleController exposes the modules.v1 registry protocol on the Terragrunt
// cache server, accepting requests authenticated with the cache server's API key
// and forwarding them to the upstream registry with the user's real credentials.
type ModuleController struct {
	*router.Router

	AuthMiddleware     router.MiddlewareFunc
	ProxyModuleHandler *handlers.ProxyModuleHandler
	Logger             log.Logger
}

// Endpoints implements controllers.Endpointer.
//
// The returned modules.v1 path is suffixed with `/` so clients performing
// service discovery against `.well-known/terraform.json` build well-formed
// module URLs. See https://developer.hashicorp.com/terraform/internals/module-registry-protocol.
func (c *ModuleController) Endpoints() map[string]any {
	return map[string]any{moduleName: c.URL().Path + "/"}
}

// Register implements router.Controller.
func (c *ModuleController) Register(r *router.Router) {
	c.Router = r.Group(modulePath)

	if c.AuthMiddleware != nil {
		c.Use(c.AuthMiddleware)
	}

	c.GET("/{registry_name}/{rest...}", c.proxyAction)
}

func (c *ModuleController) proxyAction(w router.ResponseWriter, r *http.Request) error {
	registryName := r.PathValue("registry_name")
	rest := r.PathValue("rest")

	return c.ProxyModuleHandler.Proxy(w, r, registryName, rest)
}
