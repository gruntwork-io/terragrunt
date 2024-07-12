package engine

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"

	"github.com/hashicorp/go-hclog"

	"google.golang.org/grpc/credentials/insecure"

	"github.com/gruntwork-io/go-commons/errors"
	"github.com/gruntwork-io/terragrunt-engine-go/engine"
	"github.com/gruntwork-io/terragrunt-engine-go/proto"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/util"
	"github.com/hashicorp/go-plugin"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/structpb"
)

const (
	engineVersion                   = 1
	engineCookieKey                 = "engine"
	engineCookieValue               = "terragrunt"
	EnableExperimentalEngineEnvName = "TG_EXPERIMENTAL_ENGINE"
)

var (
	engineClients = sync.Map{}
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

type engineInstance struct {
	terragruntEngine *proto.EngineClient
	client           *plugin.Client
	executionOptions *ExecutionOptions
}

// IsEngineEnabled returns true if the experimental engine is enabled.
func IsEngineEnabled() bool {
	switch strings.ToLower(os.Getenv(EnableExperimentalEngineEnvName)) {
	case "1", "yes", "true", "on":
		return true
	}
	return false
}

// Run executes the given command with the experimental engine.
func Run(
	ctx context.Context,
	runOptions *ExecutionOptions,
) (*util.CmdOutput, error) {
	workingDir := runOptions.TerragruntOptions.WorkingDir
	instance, found := engineClients.Load(workingDir)
	// initialize engine for working directory
	if !found {
		terragruntEngine, client, err := createEngine(runOptions.TerragruntOptions)
		if err != nil {
			return nil, errors.WithStackTrace(err)
		}
		engineClients.Store(workingDir, &engineInstance{
			terragruntEngine: terragruntEngine,
			client:           client,
			executionOptions: runOptions,
		})
		instance, _ = engineClients.Load(workingDir)
		if err := initialize(ctx, runOptions, terragruntEngine); err != nil {
			return nil, errors.WithStackTrace(err)
		}
	}

	engInst, ok := instance.(*engineInstance)
	if !ok {
		return nil, errors.WithStackTrace(fmt.Errorf("failed to fetch engine instance %s", workingDir))
	}
	terragruntEngine := engInst.terragruntEngine
	cmdOutput, err := invoke(ctx, runOptions, terragruntEngine)
	if err != nil {
		return nil, errors.WithStackTrace(err)
	}

	return cmdOutput, nil
}

// Shutdown shuts down the experimental engine.
func Shutdown(ctx context.Context) {
	if !IsEngineEnabled() {
		return
	}
	// iterate over all engine instances and shutdown
	engineClients.Range(func(key, value interface{}) bool {
		instance := value.(*engineInstance)
		instance.executionOptions.TerragruntOptions.Logger.Debugf("Shutting down engine for %s", instance.executionOptions.WorkingDir)
		// invoke shutdown on engine
		if err := shutdown(ctx, instance.executionOptions, instance.terragruntEngine); err != nil {
			instance.executionOptions.TerragruntOptions.Logger.Errorf("Error shutting down engine: %v", err)
		}
		// kill grpc client
		instance.client.Kill()
		return true
	})
}

// create engine for working directory
func createEngine(terragruntOptions *options.TerragruntOptions) (*proto.EngineClient, *plugin.Client, error) {
	enginePath := terragruntOptions.Engine.Source
	terragruntOptions.Logger.Debugf("Creating engine %s", enginePath)

	logger := hclog.NewInterceptLogger(&hclog.LoggerOptions{
		Level:  hclog.Debug,
		Output: terragruntOptions.Logger.Writer(),
	})
	client := plugin.NewClient(&plugin.ClientConfig{
		Logger: logger,
		HandshakeConfig: plugin.HandshakeConfig{
			ProtocolVersion:  engineVersion,
			MagicCookieKey:   engineCookieKey,
			MagicCookieValue: engineCookieValue,
		},
		Plugins: map[string]plugin.Plugin{
			"plugin": &engine.TerragruntGRPCEngine{},
		},
		Cmd: exec.Command(enginePath),
		GRPCDialOptions: []grpc.DialOption{
			grpc.WithTransportCredentials(insecure.NewCredentials()),
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

// invoke engine for working directory
func invoke(ctx context.Context, runOptions *ExecutionOptions, client *proto.EngineClient) (*util.CmdOutput, error) {
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
	// read stdout and stderr from engine
	var resultCode = 0
	for {
		runResp, err := response.Recv()
		if err != nil {
			break
		}
		if runResp == nil {
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

// initialize engine for working directory
func initialize(ctx context.Context, runOptions *ExecutionOptions, client *proto.EngineClient) error {
	terragruntOptions := runOptions.TerragruntOptions
	meta, err := convertMetaToProtobuf(runOptions.TerragruntOptions.Engine.Meta)
	if err != nil {
		return errors.WithStackTrace(err)
	}
	terragruntOptions.Logger.Debugf("Running init for engine in %s", runOptions.WorkingDir)
	request, err := (*client).Init(ctx, &proto.InitRequest{
		EnvVars:    runOptions.TerragruntOptions.Env,
		WorkingDir: runOptions.WorkingDir,
		Meta:       meta,
	})
	if err != nil {
		return errors.WithStackTrace(err)
	}
	terragruntOptions.Logger.Debugf("Reading init output for engine in %s", runOptions.WorkingDir)

	return readEngineOutput(runOptions, func() (*outputLine, error) {
		output, err := request.Recv()
		if err != nil {
			return nil, err
		}
		if output == nil {
			return nil, nil
		}
		return &outputLine{
			Stderr: output.Stderr,
			Stdout: output.Stdout,
		}, nil
	})
}

// shutdown engine for working directory
func shutdown(ctx context.Context, runOptions *ExecutionOptions, terragruntEngine *proto.EngineClient) error {
	terragruntOptions := runOptions.TerragruntOptions

	meta, err := convertMetaToProtobuf(runOptions.TerragruntOptions.Engine.Meta)
	if err != nil {
		return errors.WithStackTrace(err)
	}
	request, err := (*terragruntEngine).Shutdown(ctx, &proto.ShutdownRequest{
		WorkingDir: runOptions.WorkingDir,
		Meta:       meta,
		EnvVars:    runOptions.TerragruntOptions.Env,
	})
	if err != nil {
		return errors.WithStackTrace(err)
	}
	terragruntOptions.Logger.Debugf("Reading shutdown output for engine in %s", runOptions.WorkingDir)

	return readEngineOutput(runOptions, func() (*outputLine, error) {
		output, err := request.Recv()
		if err != nil {
			return nil, err
		}
		if output == nil {
			return nil, nil
		}
		return &outputLine{
			Stdout: output.Stdout,
			Stderr: output.Stderr,
		}, nil
	})

}

// common engine output
type outputLine struct {
	Stdout string
	Stderr string
}

type outputFn func() (*outputLine, error)

// readEngineOutput reads the output from the engine, since grpc plugins don't have common type,
// use lambda function to read bytes from the stream
func readEngineOutput(runOptions *ExecutionOptions, output outputFn) error {
	cmdStdout := runOptions.CmdStdout
	cmdStderr := runOptions.CmdStderr
	for {
		response, err := output()
		if response == nil || err != nil {
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

// convert metadata map to protobuf map
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
