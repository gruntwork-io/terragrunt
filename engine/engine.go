package engine

import (
	"bytes"
	"context"
	"encoding/json"
	goErrors "errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"

	"github.com/hashicorp/go-getter"
	"github.com/mholt/archiver/v3"

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
	engineVersion                                    = 1
	engineCookieKey                                  = "engine"
	engineCookieValue                                = "terragrunt"
	EnableExperimentalEngineEnvName                  = "TG_EXPERIMENTAL_ENGINE"
	DefaultCacheDir                                  = ".cache"
	EngineCacheDir                                   = "terragrunt/plugins/iac-engine"
	PrefixTrim                                       = "terragrunt-"
	FileNameFormat                                   = "terragrunt-iac-%s_%s_%s_%s_%s"
	ChecksumFileNameFormat                           = "terragrunt-iac-%s_%s_%s_SHA256SUMS"
	EngineCachePathEnv                               = "TG_ENGINE_CACHE_PATH"
	EngineSkipCheckEnv                               = "TG_ENGINE_SKIP_CHECK"
	TerraformCommandContextKey      engineClientsKey = iota
	LocksContextKey                 engineLocksKey   = iota
)

type engineClientsKey byte
type engineLocksKey byte

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

// Run executes the given command with the experimental engine.
func Run(
	ctx context.Context,
	runOptions *ExecutionOptions,
) (*util.CmdOutput, error) {
	engineClients, err := engineClientsFromContext(ctx)
	if err != nil {
		return nil, errors.WithStackTrace(err)
	}
	workingDir := runOptions.TerragruntOptions.WorkingDir
	instance, found := engineClients.Load(workingDir)
	// initialize engine for working directory
	if !found {
		// download engine if not available
		if err = DownloadEngine(ctx, runOptions.TerragruntOptions); err != nil {
			return nil, errors.WithStackTrace(err)
		}
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

// WithEngineValues add to context default values for engine.
func WithEngineValues(ctx context.Context) context.Context {
	if !IsEngineEnabled() {
		return ctx
	}
	ctx = context.WithValue(ctx, TerraformCommandContextKey, &sync.Map{})
	ctx = context.WithValue(ctx, LocksContextKey, util.NewKeyLocks())
	return ctx
}

// DownloadEngine downloads the engine for the given options.
func DownloadEngine(ctx context.Context, opts *options.TerragruntOptions) error {
	if !IsEngineEnabled() {
		return nil
	}
	e := opts.Engine

	if util.FileExists(e.Source) {
		// if source is a file, no need to download, exit
		return nil
	}
	path, err := engineDir(e)
	if err != nil {
		return errors.WithStackTrace(err)
	}
	if err := util.EnsureDirectory(path); err != nil {
		return errors.WithStackTrace(err)
	}
	localEngineFile := filepath.Join(path, engineFileName(e))

	// lock downloading process for only one instance
	locks, err := downloadLocksFromContext(ctx)
	if err != nil {
		return errors.WithStackTrace(err)
	}
	// locking by file where engine is downloaded
	// however, it will not help in case of multiple parallel Terragrunt runs
	locks.Lock(localEngineFile)
	defer locks.Unlock(localEngineFile)

	if util.FileExists(localEngineFile) {
		return nil
	}
	downloadFile := filepath.Join(path, enginePackageName(e))

	downloads := make(map[string]string)
	checksumFile := ""
	checksumSigFile := ""
	if strings.Contains(e.Source, "://") {
		// if source starts with absolute path, download as is
		downloads[e.Source] = downloadFile
	} else {
		baseURL := fmt.Sprintf("https://%s/releases/download/%s", e.Source, e.Version)

		// URLs and their corresponding local paths
		checksumFile = filepath.Join(path, engineChecksumName(e))
		checksumSigFile = filepath.Join(path, engineChecksumSigName(e))
		downloads[fmt.Sprintf("%s/%s", baseURL, enginePackageName(e))] = downloadFile
		downloads[fmt.Sprintf("%s/%s", baseURL, engineChecksumName(e))] = checksumFile
		downloads[fmt.Sprintf("%s/%s.sig", baseURL, engineChecksumName(e))] = checksumSigFile
	}

	for url, path := range downloads {
		opts.Logger.Infof("Downloading %s to %s", url, path)
		client := &getter.Client{
			Ctx:           ctx,
			Src:           url,
			Dst:           path,
			Mode:          getter.ClientModeFile,
			Decompressors: map[string]getter.Decompressor{},
		}

		if err := client.Get(); err != nil {
			return errors.WithStackTrace(err)
		}
	}

	if !skipEngineCheck() && checksumFile != "" && checksumSigFile != "" {
		opts.Logger.Infof("Verifying checksum for %s", downloadFile)
		if err := verifyFile(downloadFile, checksumFile, checksumSigFile); err != nil {
			return errors.WithStackTrace(err)
		}
	} else {
		opts.Logger.Warnf("Skipping verification for %s", downloadFile)
	}

	if err := extractArchive(opts, downloadFile, localEngineFile); err != nil {
		return errors.WithStackTrace(err)
	}
	opts.Logger.Infof("Engine available as %s", path)
	return nil
}

func extractArchive(opts *options.TerragruntOptions, downloadFile string, engineFile string) error {
	if !isArchiveByHeader(downloadFile) {
		opts.Logger.Info("Downloaded file is not an archive, no extraction needed")
		// move file directly if it is not an archive
		if err := os.Rename(downloadFile, engineFile); err != nil {
			return errors.WithStackTrace(err)
		}
		return nil
	}
	// extract package and process files
	path := filepath.Dir(engineFile)
	tempDir, err := os.MkdirTemp(path, "temp-")
	if err != nil {
		return errors.WithStackTrace(err)
	}
	defer func() {
		if err := os.RemoveAll(tempDir); err != nil {
			opts.Logger.Warnf("Failed to clean temp dir %s: %v", tempDir, err)
		}
	}()
	// extract archive
	if err := archiver.Unarchive(downloadFile, tempDir); err != nil {
		return errors.WithStackTrace(err)
	}
	// process files
	files, err := os.ReadDir(tempDir)
	if err != nil {
		return errors.WithStackTrace(err)
	}
	opts.Logger.Infof("Engine extracted to %s", path)

	if len(files) == 1 && !files[0].IsDir() {
		// handle case where archive contains a single file, most of the cases
		singleFile := filepath.Join(tempDir, files[0].Name())
		if err := os.Rename(singleFile, engineFile); err != nil {
			return errors.WithStackTrace(err)
		}
		return nil
	}
	// Move all files to the engine directory
	for _, file := range files {
		srcPath := filepath.Join(tempDir, file.Name())
		dstPath := filepath.Join(path, file.Name())
		if err := os.Rename(srcPath, dstPath); err != nil {
			return errors.WithStackTrace(err)
		}
	}
	return nil
}

// engineDir returns the directory path where engine files are stored.
func engineDir(e *options.EngineOptions) (string, error) {
	if util.FileExists(e.Source) {
		return filepath.Dir(e.Source), nil
	}
	cacheDir := os.Getenv(EngineCachePathEnv)
	if cacheDir == "" {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return "", errors.WithStackTrace(err)
		}
		cacheDir = filepath.Join(homeDir, DefaultCacheDir)
	}
	platform := runtime.GOOS
	arch := runtime.GOARCH
	return filepath.Join(cacheDir, EngineCacheDir, e.Type, e.Version, platform, arch), nil
}

// engineFileName returns the file name for the engine.
func engineFileName(e *options.EngineOptions) string {
	engineName := filepath.Base(e.Source)
	if util.FileExists(e.Source) {
		// return file name if source is absolute path
		return engineName
	}
	platform := runtime.GOOS
	arch := runtime.GOARCH
	engineName = strings.TrimPrefix(engineName, PrefixTrim)
	return fmt.Sprintf(FileNameFormat, engineName, e.Type, e.Version, platform, arch)
}

// engineChecksumName returns the file name of engine checksum file
func engineChecksumName(e *options.EngineOptions) string {
	engineName := filepath.Base(e.Source)

	engineName = strings.TrimPrefix(engineName, PrefixTrim)
	return fmt.Sprintf(ChecksumFileNameFormat, engineName, e.Type, e.Version)
}

// engineChecksumSigName returns the file name of engine checksum file signature
func engineChecksumSigName(e *options.EngineOptions) string {
	return engineChecksumName(e) + ".sig"
}

// enginePackageName returns the package name for the engine.
func enginePackageName(e *options.EngineOptions) string {
	return engineFileName(e) + ".zip"
}

// isArchiveByHeader checks if a file is an archive by examining its header.
func isArchiveByHeader(filePath string) bool {
	f, err := os.Open(filePath)
	if err != nil {
		return false
	}
	defer f.Close()

	archiveType, err := archiver.ByHeader(f)
	return err == nil && archiveType != nil
}

// engineClientsFromContext returns the engine clients map from the context.
func engineClientsFromContext(ctx context.Context) (*sync.Map, error) {
	val := ctx.Value(TerraformCommandContextKey)
	if val == nil {
		return nil, errors.WithStackTrace(goErrors.New("failed to fetch engine clients from context"))
	}
	result, ok := val.(*sync.Map)
	if !ok {
		return nil, errors.WithStackTrace(goErrors.New("failed to cast engine clients from context"))
	}
	return result, nil
}

// downloadLocksFromContext returns the locks map from the context.
func downloadLocksFromContext(ctx context.Context) (*util.KeyLocks, error) {
	val := ctx.Value(LocksContextKey)
	if val == nil {
		return nil, errors.WithStackTrace(goErrors.New("failed to fetch engine clients from context"))
	}
	result, ok := val.(*util.KeyLocks)
	if !ok {
		return nil, errors.WithStackTrace(goErrors.New("failed to cast engine clients from context"))
	}
	return result, nil
}

// IsEngineEnabled returns true if the experimental engine is enabled.
func IsEngineEnabled() bool {
	ok, _ := strconv.ParseBool(os.Getenv(EnableExperimentalEngineEnvName)) //nolint:errcheck
	return ok
}

// Shutdown shuts down the experimental engine.
func Shutdown(ctx context.Context) error {
	if !IsEngineEnabled() {
		return nil
	}
	// iterate over all engine instances and shutdown
	engineClients, err := engineClientsFromContext(ctx)
	if err != nil {
		return errors.WithStackTrace(err)
	}
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
	return nil
}

// createEngine create engine for working directory
func createEngine(terragruntOptions *options.TerragruntOptions) (*proto.EngineClient, *plugin.Client, error) {
	path, err := engineDir(terragruntOptions.Engine)
	if err != nil {
		return nil, nil, errors.WithStackTrace(err)
	}
	localEnginePath := filepath.Join(path, engineFileName(terragruntOptions.Engine))
	localChecksumFile := filepath.Join(path, engineChecksumName(terragruntOptions.Engine))
	localChecksumSigFile := filepath.Join(path, engineChecksumSigName(terragruntOptions.Engine))
	// validate engine before loading if verification is not disabled
	if !skipEngineCheck() && util.FileExists(localEnginePath) && util.FileExists(localChecksumFile) && util.FileExists(localChecksumSigFile) {
		if err := verifyFile(localEnginePath, localChecksumFile, localChecksumSigFile); err != nil {
			return nil, nil, errors.WithStackTrace(err)
		}
	} else {
		terragruntOptions.Logger.Warnf("Skipping verification for %s", localEnginePath)
	}
	terragruntOptions.Logger.Debugf("Creating engine %s", localEnginePath)

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
		Cmd: exec.Command(localEnginePath),
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

	meta, err := ConvertMetaToProtobuf(runOptions.TerragruntOptions.Engine.Meta)
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

	var stdoutBuf, stderrBuf bytes.Buffer
	stdout := io.MultiWriter(runOptions.CmdStdout, &stdoutBuf)
	stderr := io.MultiWriter(runOptions.CmdStderr, &stderrBuf)

	var stdoutLineBuf, stderrLineBuf bytes.Buffer
	var resultCode int

	for {
		runResp, err := response.Recv()
		if err != nil || runResp == nil {
			break
		}
		if err := processStream(runResp.GetStdout(), &stdoutLineBuf, stdout); err != nil {
			return nil, errors.WithStackTrace(err)
		}
		if err := processStream(runResp.GetStderr(), &stderrLineBuf, stderr); err != nil {
			return nil, errors.WithStackTrace(err)
		}
		resultCode = int(runResp.GetResultCode())
	}
	if err := flushBuffer(&stdoutLineBuf, stdout); err != nil {
		return nil, errors.WithStackTrace(err)
	}
	if err := flushBuffer(&stderrLineBuf, stderr); err != nil {
		return nil, errors.WithStackTrace(err)
	}

	terragruntOptions.Logger.Debugf("Engine execution done in %v", terragruntOptions.WorkingDir)

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

// processStream handles the character buffering and line printing for a given stream
func processStream(data string, lineBuf *bytes.Buffer, output io.Writer) error {
	for _, ch := range data {
		lineBuf.WriteByte(byte(ch))
		if ch == '\n' {
			if _, err := fmt.Fprint(output, lineBuf.String()); err != nil {
				return errors.WithStackTrace(err)
			}
			lineBuf.Reset()
		}
	}
	return nil
}

// flushBuffer prints any remaining data in the buffer
func flushBuffer(lineBuf *bytes.Buffer, output io.Writer) error {
	if lineBuf.Len() > 0 {
		_, err := fmt.Fprint(output, lineBuf.String())
		if err != nil {
			return errors.WithStackTrace(err)
		}
	}
	return nil
}

// initialize engine for working directory
func initialize(ctx context.Context, runOptions *ExecutionOptions, client *proto.EngineClient) error {
	terragruntOptions := runOptions.TerragruntOptions
	meta, err := ConvertMetaToProtobuf(runOptions.TerragruntOptions.Engine.Meta)
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

	return ReadEngineOutput(runOptions, func() (*OutputLine, error) {
		output, err := request.Recv()
		if err != nil {
			return nil, err
		}
		if output == nil {
			return nil, nil
		}
		return &OutputLine{
			Stderr: output.GetStderr(),
			Stdout: output.GetStdout(),
		}, nil
	})
}

// shutdown engine for working directory
func shutdown(ctx context.Context, runOptions *ExecutionOptions, terragruntEngine *proto.EngineClient) error {
	terragruntOptions := runOptions.TerragruntOptions

	meta, err := ConvertMetaToProtobuf(runOptions.TerragruntOptions.Engine.Meta)
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

	return ReadEngineOutput(runOptions, func() (*OutputLine, error) {
		output, err := request.Recv()
		if err != nil {
			return nil, err
		}
		if output == nil {
			return nil, nil
		}
		return &OutputLine{
			Stdout: output.GetStdout(),
			Stderr: output.GetStderr(),
		}, nil
	})

}

// common engine output
type OutputLine struct {
	Stdout string
	Stderr string
}

type outputFn func() (*OutputLine, error)

// ReadEngineOutput reads the output from the engine, since grpc plugins don't have common type,
// use lambda function to read bytes from the stream
func ReadEngineOutput(runOptions *ExecutionOptions, output outputFn) error {
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
	// TODO: Why does this lint need to be ignored?
	return nil //nolint:nilerr
}

// convert metadata map to protobuf map
func ConvertMetaToProtobuf(meta map[string]interface{}) (map[string]*anypb.Any, error) {
	protoMeta := make(map[string]*anypb.Any)
	if meta == nil {
		return protoMeta, nil
	}
	for key, value := range meta {
		jsonData, err := json.Marshal(value)
		if err != nil {
			return nil, fmt.Errorf("error marshaling value to JSON: %w", err)
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

// skipChecksumCheck returns true if the engine checksum check is skipped.
func skipEngineCheck() bool {
	ok, _ := strconv.ParseBool(os.Getenv(EngineSkipCheckEnv)) //nolint:errcheck
	return ok
}
