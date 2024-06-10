package main

import (
	"bufio"
	"context"
	"fmt"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	pb "github.com/gruntwork-io/terragrunt/plugins"
	"github.com/hashicorp/go-plugin"
	"google.golang.org/grpc"
	"os/exec"
)

type TofuCommandExecutor struct {
	pb.UnimplementedCommandExecutorServer
}

func (c *TofuCommandExecutor) Init(req *pb.InitRequest, stream pb.CommandExecutor_InitServer) error {
	log.Infof("Init Tofu plugin")
	err := stream.Send(&pb.InitResponse{Stdout: "Tofu Initialization started", Stderr: "", ResultCode: 0})
	if err != nil {
		return err
	}

	// Stream some metadata as stdout for demonstration
	for key, value := range req.Metadata {
		err := stream.Send(&pb.InitResponse{Stdout: fmt.Sprintf("Tofu Metadata: %s = %s", key, value), Stderr: "", ResultCode: 0})
		if err != nil {
			return err
		}
	}

	err = stream.Send(&pb.InitResponse{Stdout: "Tofu Initialization completed", Stderr: "", ResultCode: 0})
	if err != nil {
		return err
	}
	return nil
}

func (c *TofuCommandExecutor) Run(req *pb.RunRequest, stream pb.CommandExecutor_RunServer) error {
	log.Infof("Run Tofu plugin")

	cmd := exec.Command(req.Command, req.Args...)
	cmd.Dir = req.WorkingDir

	// Set environment variables
	env := make([]string, 0, len(req.EnvVars))
	for key, value := range req.EnvVars {
		env = append(env, fmt.Sprintf("%s=%s", key, value))
	}
	cmd.Env = append(cmd.Env, env...)

	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		return err
	}

	if req.AllocatePseudoTty {
		// Allocate a pseudo-TTY
	}

	if err := cmd.Run(); err != nil {
		return err
	}

	stdoutScanner := bufio.NewScanner(stdoutPipe)
	stderrScanner := bufio.NewScanner(stderrPipe)

	// Stream stdout
	go func() {
		for stdoutScanner.Scan() {
			err := stream.Send(&pb.RunResponse{
				Stdout: stdoutScanner.Text(),
			})
			if err != nil {
				return
			}
		}
	}()

	// Stream stderr
	go func() {
		for stderrScanner.Scan() {
			err := stream.Send(&pb.RunResponse{
				Stderr: stderrScanner.Text(),
			})
			if err != nil {
				return
			}
		}
	}()

	err = cmd.Wait()
	resultCode := 0
	if err != nil {
		if exitError, ok := err.(*exec.ExitError); ok {
			resultCode = exitError.ExitCode()
		} else {
			resultCode = 1
		}
	}

	err = stream.Send(&pb.RunResponse{
		ResultCode: int32(resultCode),
	})
	if err != nil {
		return err
	}

	return nil
}

// GRPCServer is used to register the TofuCommandExecutor with the gRPC server
func (c *TofuCommandExecutor) GRPCServer(broker *plugin.GRPCBroker, s *grpc.Server) error {
	pb.RegisterCommandExecutorServer(s, c)
	return nil
}

// GRPCClient is used to create a client that connects to the TofuCommandExecutor
func (c *TofuCommandExecutor) GRPCClient(ctx context.Context, broker *plugin.GRPCBroker, client *grpc.ClientConn) (interface{}, error) {
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
			"tofu": &pb.TerragruntGRPCPlugin{Impl: &TofuCommandExecutor{}},
		},
		GRPCServer: plugin.DefaultGRPCServer,
	})
}
