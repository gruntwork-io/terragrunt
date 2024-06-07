package main

import (
	"context"
	"fmt"
	pb "github.com/gruntwork-io/terragrunt/plugins"
	"github.com/hashicorp/go-plugin"
	"google.golang.org/grpc"
	"os/exec"
)

type TerraformCommandExecutor struct {
	pb.UnimplementedCommandExecutorServer
}

func (c *TerraformCommandExecutor) Init(ctx context.Context, req *pb.InitRequest) (*pb.InitResponse, error) {

	return &pb.InitResponse{ResultCode: 0}, nil
}

func (c *TerraformCommandExecutor) Run(ctx context.Context, req *pb.RunRequest) (*pb.RunResponse, error) {
	cmd := exec.Command(req.Command)
	cmd.Dir = req.WorkingDir

	// Set environment variables
	env := make([]string, 0, len(req.EnvVars))
	for key, value := range req.EnvVars {
		env = append(env, fmt.Sprintf("%s=%s", key, value))
	}
	cmd.Env = append(cmd.Env, env...)

	if req.AllocatePseudoTty {
	}
	stdout, err := cmd.Output()
	stderr := ""
	resultCode := 0
	if err != nil {
		if exitError, ok := err.(*exec.ExitError); ok {
			stderr = string(exitError.Stderr)
			resultCode = exitError.ExitCode()
		} else {
			stderr = err.Error()
			resultCode = 1
		}
	}
	return &pb.RunResponse{Stdout: string(stdout), Stderr: stderr, ResultCode: int32(resultCode)}, nil
}

// GRPCServer is used to register the TerraformCommandExecutor with the gRPC server
func (c *TerraformCommandExecutor) GRPCServer(broker *plugin.GRPCBroker, s *grpc.Server) error {
	pb.RegisterCommandExecutorServer(s, c)
	return nil
}

// GRPCClient is used to create a client that connects to the TerraformCommandExecutor
func (c *TerraformCommandExecutor) GRPCClient(ctx context.Context, broker *plugin.GRPCBroker, client *grpc.ClientConn) (interface{}, error) {
	return pb.NewCommandExecutorClient(client), nil
}

func main() {
	plugin.Serve(&plugin.ServeConfig{
		HandshakeConfig: plugin.HandshakeConfig{
			ProtocolVersion:  1,
			MagicCookieKey:   "plugin",
			MagicCookieValue: "terragrunt",
		},
		Plugins: map[string]plugin.Plugin{
			"terraform": &pb.TerragruntGRPCPlugin{Impl: &TerraformCommandExecutor{}},
		},
		GRPCServer: plugin.DefaultGRPCServer,
	})
}
