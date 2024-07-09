package engine

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/gruntwork-io/go-commons/errors"
	"github.com/gruntwork-io/terragrunt-engine-go/engine"
	"github.com/gruntwork-io/terragrunt-engine-go/proto"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/util"
	"github.com/hashicorp/go-plugin"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/structpb"
	"io"
	"os/exec"
)

type ExecutionOptions struct {
	TerragruntOptions *options.TerragruntOptions
	CmdStdout         io.Writer
	CmdStderr         io.Writer
	WorkingDir        string
	SuppressStdout    bool
	AllocatePseudoTty bool
	Command           string
	Args              []string
}

func RunEngine(
	ctx context.Context,
	runOptions *ExecutionOptions,
) (*util.CmdOutput, error) {
	terragruntEngine, client, err := createEngine(runOptions.TerragruntOptions)
	defer client.Kill()
	if err != nil {
		return nil, errors.WithStackTrace(err)
	}

	if err := initialize(ctx, runOptions, terragruntEngine); err != nil {
		return nil, errors.WithStackTrace(err)
	}

	cmdOutput, err := run(ctx, runOptions, terragruntEngine)
	if err != nil {
		return nil, errors.WithStackTrace(err)
	}

	if err := shutdown(ctx, runOptions, terragruntEngine); err != nil {
		return nil, errors.WithStackTrace(err)
	}

	return cmdOutput, nil
}

func createEngine(terragruntOptions *options.TerragruntOptions) (*proto.EngineClient, *plugin.Client, error) {
	enginePath := terragruntOptions.Engine.Source
	terragruntOptions.Logger.Debugf("Creating engine %s", enginePath)
	client := plugin.NewClient(&plugin.ClientConfig{
		HandshakeConfig: plugin.HandshakeConfig{
			ProtocolVersion: 1,
			// TODO: extract to constant
			MagicCookieKey:   "engine",
			MagicCookieValue: "terragrunt",
		},
		Plugins: map[string]plugin.Plugin{
			"plugin": &engine.TerragruntGRPCEngine{},
		},
		Cmd: exec.Command(enginePath),
		GRPCDialOptions: []grpc.DialOption{
			// TODO: use alternative for insecure
			grpc.WithInsecure(),
		},
		AllowedProtocols: []plugin.Protocol{plugin.ProtocolGRPC},
	})

	rpcClient, err := client.Client()
	if err != nil {
		return nil, nil, errors.WithStackTrace(err)
	}
	rawClient, err := rpcClient.Dispense("plugin")
	if err != nil {
		return nil, nil, errors.WithStackTrace(err)
	}

	terragruntEngine := rawClient.(proto.EngineClient)
	return &terragruntEngine, client, nil
}

func run(ctx context.Context, runOptions *ExecutionOptions, client *proto.EngineClient) (*util.CmdOutput, error) {
	terragruntOptions := runOptions.TerragruntOptions

	meta, err := convertMetaToProtobuf(runOptions.TerragruntOptions.Engine.Meta)
	if err != nil {
		return nil, errors.WithStackTrace(err)
	}

	response, err := (*client).Run(ctx, &proto.RunRequest{
		Command:           runOptions.Command,
		Args:              runOptions.Args,
		AllocatePseudoTty: runOptions.AllocatePseudoTty,
		WorkingDir:        runOptions.WorkingDir,
		Meta:              meta,
		EnvVars:           runOptions.TerragruntOptions.Env,
	})
	if err != nil {
		return nil, errors.WithStackTrace(err)
	}

	cmdStdout := runOptions.CmdStdout
	cmdStderr := runOptions.CmdStderr

	var stdoutBuf bytes.Buffer
	var stderrBuf bytes.Buffer

	stdout := io.MultiWriter(cmdStdout, &stdoutBuf)
	stderr := io.MultiWriter(cmdStderr, &stderrBuf)

	var resultCode = 0
	for {
		runResp, err := response.Recv()
		if err != nil {
			break
		}
		if runResp.Stdout != "" {
			_, err := stdout.Write([]byte(runResp.Stdout))
			if err != nil {
				return nil, errors.WithStackTrace(err)
			}
		}
		if runResp.Stderr != "" {
			_, err := stderr.Write([]byte(runResp.Stderr))
			if err != nil {
				return nil, errors.WithStackTrace(err)
			}
		}
		resultCode = int(runResp.ResultCode)
	}
	terragruntOptions.Logger.Debugf("Plugin execution done in %v", terragruntOptions.WorkingDir)

	if resultCode != 0 {
		err = util.ProcessExecutionError{
			Err:        fmt.Errorf("command failed with exit code %d", resultCode),
			StdOut:     stdoutBuf.String(),
			Stderr:     stderrBuf.String(),
			WorkingDir: terragruntOptions.WorkingDir,
		}
		return nil, errors.WithStackTrace(err)
	}

	cmdOutput := util.CmdOutput{
		Stdout: stdoutBuf.String(),
		Stderr: stderrBuf.String(),
	}

	return &cmdOutput, nil
}

func initialize(ctx context.Context, runOptions *ExecutionOptions, client *proto.EngineClient) error {
	terragruntOptions := runOptions.TerragruntOptions
	meta, err := convertMetaToProtobuf(runOptions.TerragruntOptions.Engine.Meta)
	if err != nil {
		return errors.WithStackTrace(err)
	}
	terragruntOptions.Logger.Debugf("Running init for engine in %s", runOptions.WorkingDir)
	result, err := (*client).Init(ctx, &proto.InitRequest{
		WorkingDir: runOptions.WorkingDir,
		EnvVars:    runOptions.TerragruntOptions.Env,
		Meta:       meta,
	})
	if err != nil {
		return errors.WithStackTrace(err)
	}
	terragruntOptions.Logger.Debugf("Reading init output for engine in %s", runOptions.WorkingDir)
	// read init output

	cmdStdout := runOptions.CmdStdout
	cmdStderr := runOptions.CmdStderr

	for {
		response, err := result.Recv()
		if err != nil {
			break
		}
		if response.Stdout != "" {
			_, err := cmdStdout.Write([]byte(response.Stdout))
			if err != nil {
				return errors.WithStackTrace(err)
			}
		}
		if response.Stderr != "" {
			_, err := cmdStderr.Write([]byte(response.Stderr))
			if err != nil {
				return errors.WithStackTrace(err)
			}
		}
	}
	return nil
}

func shutdown(ctx context.Context, runOptions *ExecutionOptions, terragruntEngine *proto.EngineClient) error {
	terragruntOptions := runOptions.TerragruntOptions

	meta, err := convertMetaToProtobuf(runOptions.TerragruntOptions.Engine.Meta)
	if err != nil {
		return errors.WithStackTrace(err)
	}
	result, err := (*terragruntEngine).Shutdown(ctx, &proto.ShutdownRequest{
		WorkingDir: runOptions.WorkingDir,
		EnvVars:    runOptions.TerragruntOptions.Env,
		Meta:       meta,
	})
	if err != nil {
		return errors.WithStackTrace(err)
	}
	terragruntOptions.Logger.Debugf("Reading shutdown output for engine in %s", runOptions.WorkingDir)

	cmdStdout := runOptions.CmdStdout
	cmdStderr := runOptions.CmdStderr
	for {
		response, err := result.Recv()
		if err != nil {
			break
		}
		if response.Stdout != "" {
			_, err := cmdStdout.Write([]byte(response.Stdout))
			if err != nil {
				return errors.WithStackTrace(err)
			}
		}
		if response.Stderr != "" {
			_, err := cmdStderr.Write([]byte(response.Stderr))
			if err != nil {
				return errors.WithStackTrace(err)
			}
		}
	}

	return nil
}

// TODO: investigate better env vars conversion
func convertMetaToProtobuf(meta map[string]interface{}) (map[string]*anypb.Any, error) {
	protoMeta := make(map[string]*anypb.Any)
	if meta == nil {
		return protoMeta, nil
	}
	for key, value := range meta {
		jsonData, err := json.Marshal(value)
		if err != nil {
			return nil, fmt.Errorf("error marshaling value to JSON: %v", err)
		}
		jsonStructValue, err := structpb.NewValue(string(jsonData))
		if err != nil {
			return nil, err
		}
		v, err := anypb.New(jsonStructValue)
		if err != nil {
			return nil, err
		}
		protoMeta[key] = v
	}
	return protoMeta, nil
}
