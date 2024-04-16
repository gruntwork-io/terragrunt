package controllers

import (
	"net/http"

	"github.com/gruntwork-io/terragrunt/terraform/cache/router"
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

// Register implements router.Controller.Register
func (controller *DiscoveryController) Register(router *router.Router) {
	router = router.Group(discoveryPath)

	// Discovery Process
	// https://developer.hashicorp.com/terraform/internals/remote-service-discovery#discovery-process
	router.GET("/terraform.json", controller.terraformAction)
}

func (controller *DiscoveryController) terraformAction(ctx echo.Context) error {
	endpoints := make(map[string]any)

	for _, endpointer := range controller.Endpointers {
		for name, urlPath := range endpointer.Endpoints() {
			endpoints[name] = urlPath
		}
	}

	return ctx.JSON(http.StatusOK, endpoints)
}
