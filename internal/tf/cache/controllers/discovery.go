package controllers

import (
	"maps"
	"net/http"

	"github.com/gruntwork-io/terragrunt/internal/tf/cache/router"
)

const (
	discoveryPath = "/.well-known"
)

type Endpointer interface {
	// Endpoints returns controller endpoints.
	Endpoints() map[string]any
}

type DiscoveryController struct {
	*router.Router

	Endpointers []Endpointer
}

// Register implements router.Controller.Register
func (controller *DiscoveryController) Register(router *router.Router) {
	controller.Router = router.Group(discoveryPath)

	// Discovery Process
	// https://developer.hashicorp.com/terraform/internals/remote-service-discovery#discovery-process
	controller.GET("/terraform.json", controller.terraformAction)
}

func (controller *DiscoveryController) terraformAction(w router.ResponseWriter, _ *http.Request) error {
	endpoints := make(map[string]any)

	for _, endpointer := range controller.Endpointers {
		maps.Copy(endpoints, endpointer.Endpoints())
	}

	return router.JSON(w, http.StatusOK, endpoints)
}
