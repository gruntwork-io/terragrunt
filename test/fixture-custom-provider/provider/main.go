package main

import (
	"github.com/gruntwork-io/terragrunt-custom-provider/tg"
	"github.com/hashicorp/terraform-plugin-sdk/plugin"
)

func main() {
	plugin.Serve(&plugin.ServeOpts{
		ProviderFunc: tg.Provider,
	})
}
