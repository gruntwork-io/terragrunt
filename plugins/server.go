package plugins

import (
	"net/rpc"

	"github.com/hashicorp/go-plugin"
)

type TerragruntPluginServer struct {
	Impl Plugin
}

func (s *TerragruntPluginServer) Run(args *RunCmd, resp *RunCmdResponse) error {
	result, err := s.Impl.Run(args)
	if err != nil {
		return err
	}
	*resp = *result
	return nil
}

type TerragruntPluginRPCPlugin struct {
	plugin.NetRPCUnsupportedPlugin
	Impl Plugin
}

func (p *TerragruntPluginRPCPlugin) Server(*plugin.MuxBroker) (interface{}, error) {
	return &TerragruntPluginServer{Impl: p.Impl}, nil
}

func (p *TerragruntPluginRPCPlugin) Client(b *plugin.MuxBroker, c *rpc.Client) (interface{}, error) {
	return &MyPluginRPCClient{client: c}, nil
}

type MyPluginRPCClient struct {
	client *rpc.Client
}

func (c *MyPluginRPCClient) Run(runCmd *RunCmd) (*RunCmdResponse, error) {
	var resp RunCmdResponse
	err := c.client.Call("Plugin.Run", runCmd, &resp)
	if err != nil {
		return nil, err
	}
	return &resp, nil
}
