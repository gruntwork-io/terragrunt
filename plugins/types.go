package plugins

import (
	"context"
	"github.com/hashicorp/go-plugin"
	"google.golang.org/grpc"
)

type TerragruntGRPCPlugin struct {
	plugin.Plugin
	Impl CommandExecutorServer
}

func (p *TerragruntGRPCPlugin) GRPCServer(broker *plugin.GRPCBroker, s *grpc.Server) error {
	RegisterCommandExecutorServer(s, p.Impl)
	return nil
}

func (p *TerragruntGRPCPlugin) GRPCClient(ctx context.Context, broker *plugin.GRPCBroker, c *grpc.ClientConn) (interface{}, error) {
	return NewCommandExecutorClient(c), nil
}
