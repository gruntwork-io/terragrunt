// Package tofuengine is a Terragrunt IaC engine for tests. It runs the OpenTofu binary that
// [EnvTFPath] names, so a test can remove PATH from the process and still apply a unit through
// an engine. Terragrunt spawns the engine with its own environment, which is how the variable
// reaches it.
package tofuengine

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/gruntwork-io/terragrunt-engine-go/engine"
	"github.com/gruntwork-io/terragrunt-engine-go/proto"
	"github.com/hashicorp/go-plugin"
	"github.com/stretchr/testify/require"
	"golang.org/x/sync/errgroup"
)

// EnvTFPath names the OpenTofu binary the engine runs. It must be an absolute path: the engine
// inherits Terragrunt's environment, which the tests strip of PATH.
const EnvTFPath = "TG_TEST_TOFU_ENGINE_TF_PATH"

// chunkSize bounds how much of a stream one RunResponse carries.
const chunkSize = 4096

// Handshake matches the handshake Terragrunt's engine client expects.
var Handshake = plugin.HandshakeConfig{
	ProtocolVersion:  1,
	MagicCookieKey:   "engine",
	MagicCookieValue: "terragrunt",
}

// Build compiles the engine binary into a directory that lives as long as t, and returns its
// path. It needs a Go toolchain on PATH, so call it before removing PATH.
func Build(t *testing.T) string {
	t.Helper()

	name := "tofuengine"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}

	path := filepath.Join(t.TempDir(), name)

	cmd := exec.CommandContext(
		t.Context(),
		"go",
		"build",
		"-o",
		path,
		"github.com/gruntwork-io/terragrunt/test/helpers/tofuengine/cmd",
	)

	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "building the test engine: %s", out)

	return path
}

// Serve runs the engine as a go-plugin server until Terragrunt shuts it down.
func Serve() {
	plugin.Serve(&plugin.ServeConfig{
		HandshakeConfig: Handshake,
		Plugins: map[string]plugin.Plugin{
			"plugin": &engine.TerragruntGRPCEngine{Impl: &server{}},
		},
		GRPCServer: plugin.DefaultGRPCServer,
	})
}

// server answers Terragrunt's engine calls. Init and Shutdown have nothing to do; Run execs the
// binary [EnvTFPath] names with the request's arguments, working directory and environment.
type server struct {
	proto.UnimplementedEngineServer
}

func (s *server) Init(_ *proto.InitRequest, stream proto.Engine_InitServer) error {
	return stream.Send(&proto.InitResponse{
		Response: &proto.InitResponse_ExitResult{ExitResult: &proto.ExitResultMessage{Code: 0}},
	})
}

func (s *server) Shutdown(_ *proto.ShutdownRequest, stream proto.Engine_ShutdownServer) error {
	return stream.Send(&proto.ShutdownResponse{
		Response: &proto.ShutdownResponse_ExitResult{ExitResult: &proto.ExitResultMessage{Code: 0}},
	})
}

func (s *server) Run(req *proto.RunRequest, stream proto.Engine_RunServer) error {
	tfPath := os.Getenv(EnvTFPath)
	if tfPath == "" {
		return sendFailure(stream, fmt.Errorf("%s is not set", EnvTFPath))
	}

	cmd := exec.CommandContext(stream.Context(), tfPath, req.GetArgs()...)
	cmd.Dir = req.GetWorkingDir()

	for key, value := range req.GetEnvVars() {
		cmd.Env = append(cmd.Env, key+"="+value)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return sendFailure(stream, err)
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		return sendFailure(stream, err)
	}

	if err := cmd.Start(); err != nil {
		return sendFailure(stream, err)
	}

	chunks := make(chan *proto.RunResponse)

	readers := new(errgroup.Group)
	readers.Go(func() error {
		return forward(stdout, chunks, func(content string) *proto.RunResponse {
			return &proto.RunResponse{
				Response: &proto.RunResponse_Stdout{Stdout: &proto.StdoutMessage{Content: content}},
			}
		})
	})
	readers.Go(func() error {
		return forward(stderr, chunks, func(content string) *proto.RunResponse {
			return &proto.RunResponse{
				Response: &proto.RunResponse_Stderr{Stderr: &proto.StderrMessage{Content: content}},
			}
		})
	})

	sender := new(errgroup.Group)
	sender.Go(func() error {
		for chunk := range chunks {
			if err := stream.Send(chunk); err != nil {
				return err
			}
		}

		return nil
	})

	readErr := readers.Wait()

	close(chunks)

	if err := sender.Wait(); err != nil {
		return err
	}

	if readErr != nil {
		return readErr
	}

	code := 0

	if err := cmd.Wait(); err != nil {
		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) {
			return sendFailure(stream, err)
		}

		code = exitErr.ExitCode()
	}

	return stream.Send(&proto.RunResponse{
		Response: &proto.RunResponse_ExitResult{ExitResult: &proto.ExitResultMessage{Code: int32(code)}},
	})
}

// forward reads r to EOF, wrapping each chunk with wrap and passing it on chunks.
func forward(r io.Reader, chunks chan<- *proto.RunResponse, wrap func(string) *proto.RunResponse) error {
	reader := bufio.NewReader(r)
	buf := make([]byte, chunkSize)

	for {
		n, err := reader.Read(buf)
		if n > 0 {
			chunks <- wrap(string(buf[:n]))
		}

		if errors.Is(err, io.EOF) {
			return nil
		}

		if err != nil {
			return err
		}
	}
}

// sendFailure reports err on the stream as stderr followed by a failing exit result.
func sendFailure(stream proto.Engine_RunServer, err error) error {
	if sendErr := stream.Send(&proto.RunResponse{
		Response: &proto.RunResponse_Stderr{Stderr: &proto.StderrMessage{Content: err.Error() + "\n"}},
	}); sendErr != nil {
		return sendErr
	}

	return stream.Send(&proto.RunResponse{
		Response: &proto.RunResponse_ExitResult{ExitResult: &proto.ExitResultMessage{Code: 1}},
	})
}
