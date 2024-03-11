package controllers

import (
	"net/http"

	"github.com/gruntwork-io/terragrunt/terraform/registry/router"
	"github.com/labstack/echo/v4"
)

const (
	discoveryPath = "/.well-known"
)

type Endpointer interface {
	// Endpoints returns controller endpoints.
	Endpoints() map[string]any
}

type DiscoveryController struct {
	Endpointers []Endpointer
}

// Paths implements router.Controller.Register
func (controller *DiscoveryController) Register(router *router.Router) {
	router = router.Group(discoveryPath)

	router.GET("/terraform.json", controller.terraformAction)
}

// terraformAction represents Terraform Service Endpoints API endpoint.
// Docs: https://www.terraform.io/internals/remote-service-discovery
func (controller *DiscoveryController) terraformAction(ctx echo.Context) error {
	endpoints := make(map[string]any)

	for _, endpointer := range controller.Endpointers {
		for name, urlPath := range endpointer.Endpoints() {
			endpoints[name] = urlPath
		}
	}

	return ctx.JSON(http.StatusOK, endpoints)
}
