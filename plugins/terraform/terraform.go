package main

import (
	terragruntplugins "github.com/gruntwork-io/terragrunt/plugins"
	"github.com/hashicorp/go-plugin"
)

type TerraformPlugin struct{}

func (p *TerraformPlugin) Run(runCmd *terragruntplugins.RunCmd) (*terragruntplugins.RunCmdResponse, error) {

	// Terraform plugin logic here
	return &terragruntplugins.RunCmdResponse{}, nil
}

func main() {
	pluginMap := map[string]plugin.Plugin{
		"terraform": &terragruntplugins.TerragruntPluginRPCPlugin{Impl: &TerraformPlugin{}},
	}

	plugin.Serve(&plugin.ServeConfig{
		HandshakeConfig: plugin.HandshakeConfig{
			ProtocolVersion:  1,
			MagicCookieKey:   "terraform",
			MagicCookieValue: "terraform",
		},
		Plugins:    pluginMap,
		GRPCServer: plugin.DefaultGRPCServer,
	})
}
