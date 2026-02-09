// Package engine provides the pluggable IaC engine for Terragrunt.
package engine

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/gruntwork-io/terragrunt/pkg/log"

	"github.com/gruntwork-io/terragrunt/internal/cache"
	"github.com/gruntwork-io/terragrunt/internal/experiment"
	"github.com/gruntwork-io/terragrunt/internal/github"
	"github.com/gruntwork-io/terragrunt/internal/os/signal"

	"github.com/hashicorp/go-hclog"

	"google.golang.org/grpc/credentials/insecure"

	"github.com/gruntwork-io/terragrunt-engine-go/engine"
	"github.com/gruntwork-io/terragrunt-engine-go/proto"
	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/internal/util"
	"github.com/gruntwork-io/terragrunt/pkg/options"
	"github.com/hashicorp/go-plugin"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/structpb"
)

const (
	engineVersion     = 1
	engineCookieKey   = "engine"
	engineCookieValue = "terragrunt"

	defaultCacheDir                             = ".cache"
	defaultEngineCachePath                      = "terragrunt/plugins/iac-engine"
	prefixTrim                                  = "terragrunt-"
	fileNameFormat                              = "terragrunt-iac-%s_%s_%s_%s_%s"
	checksumFileNameFormat                      = "terragrunt-iac-%s_%s_%s_SHA256SUMS"
	engineLogLevelEnv                           = "TG_ENGINE_LOG_LEVEL"
	defaultEngineRepoRoot                       = "github.com/"
	terraformCommandContextKey engineClientsKey = iota
	locksContextKey            engineLocksKey   = iota
	latestVersionsContextKey   engineLocksKey   = iota

	dirPerm = 0755

	errMsgEngineClientsFetch = "failed to fetch engine clients from context"
	errMsgEngineClientsCast  = "failed to cast engine clients from context"
	errMsgVersionsCacheFetch = "failed to fetch engine versions cache from context"
	errMsgVersionsCacheCast  = "failed to cast engine versions cache from context"
)

type (
	engineClientsKey byte
	engineLocksKey   byte
)

type ExecutionOptions struct {
	CmdStdout         io.Writer
	CmdStderr         io.Writer
	TerragruntOptions *options.TerragruntOptions
	WorkingDir        string
	Command           string
	Args              []string
	SuppressStdout    bool
	AllocatePseudoTty bool
}

type engineInstance struct {
	terragruntEngine *proto.EngineClient
	client           *plugin.Client
	executionOptions *ExecutionOptions
}

// Run executes the given command with the experimental engine.
func Run(
	ctx context.Context,
	l log.Logger,
	runOptions *ExecutionOptions,
) (*util.CmdOutput, error) {
	engineClients, err := engineClientsFromContext(ctx)
	if err != nil {
		return nil, errors.New(err)
	}

	workingDir := runOptions.TerragruntOptions.WorkingDir
	instance, found := engineClients.Load(workingDir)
	// initialize engine for working directory
	if !found {
		// download engine if not available
		if err = DownloadEngine(ctx, l, runOptions.TerragruntOptions); err != nil {
			return nil, errors.New(err)
		}

		terragruntEngine, client, createEngineErr := createEngine(ctx, l, runOptions.TerragruntOptions)
		if createEngineErr != nil {
			return nil, errors.New(createEngineErr)
		}

		engineClients.Store(workingDir, &engineInstance{
			terragruntEngine: terragruntEngine,
			client:           client,
			executionOptions: runOptions,
		})

		instance, _ = engineClients.Load(workingDir)

		if err = initialize(ctx, l, runOptions, terragruntEngine); err != nil {
			return nil, errors.New(err)
		}
	}

	engInst, ok := instance.(*engineInstance)
	if !ok {
		return nil, errors.Errorf("failed to fetch engine instance %s", workingDir)
	}

	terragruntEngine := engInst.terragruntEngine

	output, err := invoke(ctx, l, runOptions, terragruntEngine)
	if err != nil {
		return nil, errors.New(err)
	}

	return output, nil
}

// WithEngineValues add to context default values for engine.
func WithEngineValues(ctx context.Context) context.Context {
	ctx = context.WithValue(ctx, terraformCommandContextKey, &sync.Map{})
	ctx = context.WithValue(ctx, locksContextKey, util.NewKeyLocks())
	ctx = context.WithValue(ctx, latestVersionsContextKey, cache.NewCache[string]("engineVersions"))

	return ctx
}

// DownloadEngine downloads the engine for the given options.
func DownloadEngine(ctx context.Context, l log.Logger, opts *options.TerragruntOptions) error {
	if !opts.Experiments.Evaluate(experiment.IacEngine) || opts.NoEngine {
		return nil
	}

	e := opts.Engine
	if e == nil {
		return nil
	}

	if util.FileExists(e.Source) {
		// if source is a file, no need to download, exit
		return nil
	}

	// If source is empty, we cannot download the engine
	// This indicates an engine block was configured but source was not provided
	if e.Source == "" {
		return errors.Errorf(
			"engine block is configured but source is empty. Please provide an engine source or remove the engine block",
		)
	}

	// identify engine version if not specified
	if len(e.Version) == 0 {
		if !strings.Contains(e.Source, "://") {
			tag, err := lastReleaseVersion(ctx, opts)
			if err != nil {
				return errors.New(err)
			}

			e.Version = tag
		}
	}

	path, err := engineDir(opts)
	if err != nil {
		return errors.New(err)
	}

	if ensureErr := util.EnsureDirectory(path); ensureErr != nil {
		return errors.New(ensureErr)
	}

	localEngineFile := filepath.Join(path, engineFileName(e))

	// lock downloading process for only one instance
	locks, err := downloadLocksFromContext(ctx)
	if err != nil {
		return errors.New(err)
	}
	// locking by file where engine is downloaded
	// however, it will not help in case of multiple parallel Terragrunt runs
	locks.Lock(localEngineFile)
	defer locks.Unlock(localEngineFile)

	if util.FileExists(localEngineFile) {
		return nil
	}

	downloadFile := filepath.Join(path, enginePackageName(e))

	// Prepare download assets
	assets := &github.ReleaseAssets{
		Repository:  e.Source,
		Version:     e.Version,
		PackageFile: downloadFile,
	}

	var checksumFile, checksumSigFile string

	// Only add checksum files for GitHub releases (not direct URLs)
	if !strings.Contains(e.Source, "://") {
		checksumFile = filepath.Join(path, engineChecksumName(e))
		checksumSigFile = filepath.Join(path, engineChecksumSigName(e))
		assets.ChecksumFile = checksumFile
		assets.ChecksumSigFile = checksumSigFile
	}

	// Create download client and download assets
	downloadClient := github.NewGitHubReleasesDownloadClient(github.WithLogger(l))

	result, err := downloadClient.DownloadReleaseAssets(ctx, assets)
	if err != nil {
		return errors.Errorf("failed to download engine assets: %w", err)
	}

	// Update file paths from result
	downloadFile = result.PackageFile
	checksumFile = result.ChecksumFile
	checksumSigFile = result.ChecksumSigFile

	if !opts.EngineSkipChecksumCheck && checksumFile != "" && checksumSigFile != "" {
		l.Infof("Verifying checksum for %s", downloadFile)

		if err := verifyFile(downloadFile, checksumFile, checksumSigFile); err != nil {
			return errors.New(err)
		}
	} else {
		l.Warnf("Skipping verification for %s", downloadFile)
	}

	if err := extractArchive(l, downloadFile, localEngineFile); err != nil {
		return errors.New(err)
	}

	l.Infof("Engine available as %s", path)

	return nil
}

func lastReleaseVersion(ctx context.Context, opts *options.TerragruntOptions) (string, error) {
	repository := strings.TrimPrefix(opts.Engine.Source, defaultEngineRepoRoot)

	versionCache, err := engineVersionsCacheFromContext(ctx)
	if err != nil {
		return "", errors.New(err)
	}

	cacheKey := "github_release_" + repository
	if val, found := versionCache.Get(ctx, cacheKey); found {
		return val, nil
	}

	githubClient := github.NewGitHubAPIClient(github.WithGithubComDefaultAuth())

	tag, err := githubClient.GetLatestReleaseTag(ctx, repository)
	if err != nil {
		return "", errors.Errorf("failed to get latest release for repository %s: %w", repository, err)
	}

	versionCache.Put(ctx, cacheKey, tag)

	return tag, nil
}

func extractArchive(l log.Logger, downloadFile string, engineFile string) error {
	if !isArchiveByHeader(l, downloadFile) {
		l.Info("Downloaded file is not an archive, no extraction needed")
		// move file directly if it is not an archive
		if err := os.Rename(downloadFile, engineFile); err != nil {
			return errors.New(err)
		}

		return nil
	}
	// extract package and process files
	path := filepath.Dir(engineFile)

	tempDir, err := os.MkdirTemp(path, "temp-")
	if err != nil {
		return errors.New(err)
	}

	defer func() {
		if err = os.RemoveAll(tempDir); err != nil {
			l.Warnf("Failed to clean temp dir %s: %v", tempDir, err)
		}
	}()
	// extract archive
	if err = extract(l, downloadFile, tempDir); err != nil {
		return errors.New(err)
	}

	// process files
	files, err := os.ReadDir(tempDir)
	if err != nil {
		return errors.New(err)
	}

	l.Infof("Engine extracted to %s", path)

	if len(files) == 1 && !files[0].IsDir() {
		// handle case where archive contains a single file, most of the cases
		singleFile := filepath.Join(tempDir, files[0].Name())
		if err := os.Rename(singleFile, engineFile); err != nil {
			return errors.New(err)
		}

		return nil
	}

	// Move all files to the engine directory
	for _, file := range files {
		srcPath := filepath.Join(tempDir, file.Name())

		dstPath := filepath.Join(path, file.Name())
		if err := os.Rename(srcPath, dstPath); err != nil {
			return errors.New(err)
		}
	}

	return nil
}

// engineDir returns the directory path where engine files are stored.
func engineDir(terragruntOptions *options.TerragruntOptions) (string, error) {
	engine := terragruntOptions.Engine
	if util.FileExists(engine.Source) {
		return filepath.Dir(engine.Source), nil
	}

	cacheDir := terragruntOptions.EngineCachePath
	if len(cacheDir) == 0 {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return "", errors.New(err)
		}

		cacheDir = filepath.Join(homeDir, defaultCacheDir)
	}

	platform := runtime.GOOS
	arch := runtime.GOARCH

	return filepath.Join(cacheDir, defaultEngineCachePath, engine.Type, engine.Version, platform, arch), nil
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
	engineName = strings.TrimPrefix(engineName, prefixTrim)

	return fmt.Sprintf(fileNameFormat, engineName, e.Type, e.Version, platform, arch)
}

// engineChecksumName returns the file name of engine checksum file
func engineChecksumName(e *options.EngineOptions) string {
	engineName := filepath.Base(e.Source)

	engineName = strings.TrimPrefix(engineName, prefixTrim)

	return fmt.Sprintf(checksumFileNameFormat, engineName, e.Type, e.Version)
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
func isArchiveByHeader(l log.Logger, filePath string) bool {
	archiveType, err := detectFileType(l, filePath)

	return err == nil && archiveType != ""
}

// engineClientsFromContext returns the engine clients map from the context.
func engineClientsFromContext(ctx context.Context) (*sync.Map, error) {
	val := ctx.Value(terraformCommandContextKey)
	if val == nil {
		return nil, errors.New(errMsgEngineClientsFetch)
	}

	result, ok := val.(*sync.Map)
	if !ok {
		return nil, errors.New(errMsgEngineClientsCast)
	}

	return result, nil
}

// downloadLocksFromContext returns the locks map from the context.
func downloadLocksFromContext(ctx context.Context) (*util.KeyLocks, error) {
	val := ctx.Value(locksContextKey)
	if val == nil {
		return nil, errors.New(errMsgEngineClientsFetch)
	}

	result, ok := val.(*util.KeyLocks)
	if !ok {
		return nil, errors.New(errMsgEngineClientsCast)
	}

	return result, nil
}

func engineVersionsCacheFromContext(ctx context.Context) (*cache.Cache[string], error) {
	val := ctx.Value(latestVersionsContextKey)
	if val == nil {
		return nil, errors.New(errMsgVersionsCacheFetch)
	}

	result, ok := val.(*cache.Cache[string])
	if !ok {
		return nil, errors.New(errMsgVersionsCacheCast)
	}

	return result, nil
}

const (
	gracefulExitTimeout    = 5 * time.Second
	pluginExitPollInterval = 50 * time.Millisecond
)

// Shutdown shuts down the experimental engine.
func Shutdown(ctx context.Context, l log.Logger, opts *options.TerragruntOptions) error {
	if !opts.Experiments.Evaluate(experiment.IacEngine) || opts.NoEngine {
		return nil
	}

	// iterate over all engine instances and shutdown
	engineClients, err := engineClientsFromContext(ctx)
	if err != nil {
		return errors.New(err)
	}

	engineClients.Range(func(key, value any) bool {
		instance := value.(*engineInstance)
		l.Debugf("Shutting down engine for %s", instance.executionOptions.WorkingDir)

		// We use without cancel here to ensure that the shutdown isn't cancelled by the main context,
		// like it is in the RunCommandWithOutput function. This ensures that we don't cancel the shutdown
		// when the command is cancelled.
		if err := shutdown(
			context.WithoutCancel(ctx),
			l,
			instance.executionOptions,
			instance.terragruntEngine,
		); err != nil {
			l.Errorf("Error shutting down engine: %v", err)
		}

		// Wait for plugin to exit gracefully before force-killing.
		// The shutdown RPC has already told the plugin to exit, so it should
		// be cleaning up and exiting on its own. Give it time to finish.
		if !waitForPluginExit(instance.client, gracefulExitTimeout) {
			l.Debugf("Plugin did not exit gracefully within timeout, force killing")
			instance.client.Kill()
		}

		return true
	})

	return nil
}

// waitForPluginExit waits for the plugin process to exit, returning true if it exited
// within the timeout, false otherwise.
func waitForPluginExit(client *plugin.Client, timeout time.Duration) bool {
	done := make(chan struct{})

	go func() {
		// Client.Exited() returns true when the plugin process has exited
		for !client.Exited() {
			time.Sleep(pluginExitPollInterval)
		}

		close(done)
	}()

	select {
	case <-done:
		return true
	case <-time.After(timeout):
		return false
	}
}

// logEngineMessage logs a message from the engine at the appropriate log level.
func logEngineMessage(l log.Logger, logLevel proto.LogLevel, content string) {
	switch logLevel {
	case proto.LogLevel_LOG_LEVEL_DEBUG:
		l.Debug(content)
	case proto.LogLevel_LOG_LEVEL_INFO:
		l.Info(content)
	case proto.LogLevel_LOG_LEVEL_WARN:
		l.Warn(content)
	case proto.LogLevel_LOG_LEVEL_ERROR:
		l.Error(content)
	case proto.LogLevel_LOG_LEVEL_UNSPECIFIED:
		// Treat unspecified as debug level
		l.Debug(content)
	}
}

// createEngine create engine for working directory
func createEngine(
	ctx context.Context,
	l log.Logger,
	opts *options.TerragruntOptions,
) (*proto.EngineClient, *plugin.Client, error) {
	if opts.Engine == nil {
		return nil, nil, errors.Errorf("engine options are nil")
	}

	// If source is empty, we cannot determine the engine file path
	if opts.Engine.Source == "" {
		return nil, nil, errors.Errorf("engine source is empty, cannot create engine")
	}

	path, err := engineDir(opts)
	if err != nil {
		return nil, nil, errors.New(err)
	}

	localEnginePath := filepath.Join(path, engineFileName(opts.Engine))
	localChecksumFile := filepath.Join(path, engineChecksumName(opts.Engine))
	localChecksumSigFile := filepath.Join(path, engineChecksumSigName(opts.Engine))

	// validate engine before loading if verification is not disabled
	skipCheck := opts.EngineSkipChecksumCheck
	if !skipCheck && util.FileExists(localEnginePath) && util.FileExists(localChecksumFile) &&
		util.FileExists(localChecksumSigFile) {
		if err = verifyFile(localEnginePath, localChecksumFile, localChecksumSigFile); err != nil {
			return nil, nil, errors.New(err)
		}
	} else {
		l.Warnf("Skipping verification for %s", localEnginePath)
	}

	l.Debugf("Creating engine %s", localEnginePath)

	engineLogLevel := opts.EngineLogLevel
	if len(engineLogLevel) == 0 {
		engineLogLevel = hclog.Warn.String()
		// update log level if it is different from info
		if l.Level() != log.InfoLevel {
			engineLogLevel = l.Level().String()
		}
		// turn off log formatting if disabled for Terragrunt
		if l.Formatter().DisabledOutput() {
			engineLogLevel = hclog.Off.String()
		}
	}

	logger := hclog.NewInterceptLogger(&hclog.LoggerOptions{
		Level:  hclog.LevelFromString(engineLogLevel),
		Output: l.Writer(),
	})

	// We use without cancel here to ensure that the plugin isn't killed when the main context is cancelled,
	// like it is in the RunCommandWithOutput function. This ensures that we don't cancel the shutdown
	// when the command is cancelled.
	cmd := exec.CommandContext(
		context.WithoutCancel(ctx),
		localEnginePath,
	)
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}

		if sig := signal.SignalFromContext(ctx); sig != nil {
			return cmd.Process.Signal(sig)
		}

		return cmd.Process.Signal(os.Kill)
	}
	// pass log level to engine
	cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", engineLogLevelEnv, engineLogLevel))
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
		Cmd: cmd,
		GRPCDialOptions: []grpc.DialOption{
			grpc.WithTransportCredentials(insecure.NewCredentials()),
		},
		AllowedProtocols: []plugin.Protocol{plugin.ProtocolGRPC},
	})

	rpcClient, err := client.Client()
	if err != nil {
		return nil, nil, errors.New(err)
	}

	rawClient, err := rpcClient.Dispense("plugin")
	if err != nil {
		return nil, nil, errors.New(err)
	}

	terragruntEngine := rawClient.(proto.EngineClient)

	return &terragruntEngine, client, nil
}

// invoke engine for working directory
func invoke(ctx context.Context, l log.Logger, runOptions *ExecutionOptions, client *proto.EngineClient) (*util.CmdOutput, error) {
	opts := runOptions.TerragruntOptions

	meta, err := ConvertMetaToProtobuf(runOptions.TerragruntOptions.Engine.Meta)
	if err != nil {
		return nil, errors.New(err)
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
		return nil, errors.New(err)
	}

	var (
		output = util.CmdOutput{}

		stdout = io.MultiWriter(runOptions.CmdStdout, &output.Stdout)
		stderr = io.MultiWriter(runOptions.CmdStderr, &output.Stderr)
	)

	var (
		stdoutLineBuf, stderrLineBuf bytes.Buffer
		resultCode                   int
	)

	for {
		runResp, recvErr := response.Recv()
		if recvErr != nil || runResp == nil {
			break
		}

		responseType := runResp.GetResponse()
		if responseType == nil {
			continue
		}

		switch resp := responseType.(type) {
		case *proto.RunResponse_Stdout:
			if resp.Stdout != nil {
				if err = processStream(resp.Stdout.GetContent(), &stdoutLineBuf, stdout); err != nil {
					return nil, errors.New(err)
				}
			}
		case *proto.RunResponse_Stderr:
			if resp.Stderr != nil {
				if err = processStream(resp.Stderr.GetContent(), &stderrLineBuf, stderr); err != nil {
					return nil, errors.New(err)
				}
			}
		case *proto.RunResponse_ExitResult:
			if resp.ExitResult != nil {
				resultCode = int(resp.ExitResult.GetCode())
			}
		case *proto.RunResponse_Log:
			if resp.Log != nil {
				if logContent := resp.Log.GetContent(); logContent != "" {
					logEngineMessage(l, resp.Log.GetLevel(), logContent)
				}
			}
		}
	}

	if err = flushBuffer(&stdoutLineBuf, stdout); err != nil {
		return nil, errors.New(err)
	}

	if err = flushBuffer(&stderrLineBuf, stderr); err != nil {
		return nil, errors.New(err)
	}

	l.Debugf("Engine execution done in %v", opts.WorkingDir)

	if resultCode != 0 {
		err = util.ProcessExecutionError{
			Err:             errors.Errorf("command failed with exit code %d", resultCode),
			Output:          output,
			WorkingDir:      opts.WorkingDir,
			RootWorkingDir:  opts.RootWorkingDir,
			LogShowAbsPaths: opts.LogShowAbsPaths,
			Command:         runOptions.Command,
			Args:            runOptions.Args,
			DisableSummary:  opts.LogDisableErrorSummary,
		}

		return nil, errors.New(err)
	}

	return &output, nil
}

// processStream handles the character buffering and line printing for a given stream
func processStream(data string, lineBuf *bytes.Buffer, output io.Writer) error {
	for _, ch := range data {
		lineBuf.WriteRune(ch)

		if ch == '\n' {
			if _, err := fmt.Fprint(output, lineBuf.String()); err != nil {
				return errors.New(err)
			}

			lineBuf.Reset()
		}
	}

	return nil
}

// flushBuffer prints any remaining data in the buffer
func flushBuffer(lineBuf *bytes.Buffer, output io.Writer) error {
	if lineBuf.Len() > 0 {
		if _, err := fmt.Fprint(output, lineBuf.String()); err != nil {
			return errors.New(err)
		}
	}

	return nil
}

var ErrEngineInitFailed = errors.New("engine init failed")

// initialize engine for working directory
func initialize(ctx context.Context, l log.Logger, runOptions *ExecutionOptions, client *proto.EngineClient) error {
	meta, err := ConvertMetaToProtobuf(runOptions.TerragruntOptions.Engine.Meta)
	if err != nil {
		return errors.New(err)
	}

	l.Debugf("Running init for engine in %s", runOptions.WorkingDir)

	request, err := (*client).Init(ctx, &proto.InitRequest{
		EnvVars:    runOptions.TerragruntOptions.Env,
		WorkingDir: runOptions.WorkingDir,
		Meta:       meta,
	})
	if err != nil {
		return errors.New(err)
	}

	l.Debugf("Reading init output for engine in %s", runOptions.WorkingDir)

	return ReadEngineOutput(runOptions, true, func() (*OutputLine, error) {
		output, err := request.Recv()
		if err != nil {
			return nil, err
		}

		if output == nil {
			return nil, nil
		}

		outputLine := &OutputLine{}

		//nolint:dupl // Similar structure to shutdown response handling, but different protobuf types
		switch resp := output.GetResponse().(type) {
		case *proto.InitResponse_Stdout:
			if resp.Stdout != nil {
				outputLine.Stdout = resp.Stdout.GetContent()
			}
		case *proto.InitResponse_Stderr:
			if resp.Stderr != nil {
				outputLine.Stderr = resp.Stderr.GetContent()
			}
		case *proto.InitResponse_ExitResult:
			if resp.ExitResult != nil {
				exitCode := int(resp.ExitResult.GetCode())
				if exitCode != 0 {
					l.Errorf("Engine init failed with exit code %d", exitCode)
					return nil, errors.Errorf("%w with exit code %d", ErrEngineInitFailed, exitCode)
				}
			}
		case *proto.InitResponse_Log:
			if resp.Log != nil {
				if logContent := resp.Log.GetContent(); logContent != "" {
					logEngineMessage(l, resp.Log.GetLevel(), logContent)
				}
			}
		}

		return outputLine, nil
	})
}

var ErrEngineShutdownFailed = errors.New("engine shutdown failed")

// shutdown engine for working directory
func shutdown(ctx context.Context, l log.Logger, runOptions *ExecutionOptions, terragruntEngine *proto.EngineClient) error {
	meta, err := ConvertMetaToProtobuf(runOptions.TerragruntOptions.Engine.Meta)
	if err != nil {
		return errors.New(err)
	}

	request, err := (*terragruntEngine).Shutdown(ctx, &proto.ShutdownRequest{
		WorkingDir: runOptions.WorkingDir,
		Meta:       meta,
		EnvVars:    runOptions.TerragruntOptions.Env,
	})
	if err != nil {
		return errors.New(err)
	}

	l.Debugf("Reading shutdown output for engine in %s", runOptions.WorkingDir)

	return ReadEngineOutput(runOptions, true, func() (*OutputLine, error) {
		output, err := request.Recv()
		if err != nil {
			return nil, err
		}

		if output == nil {
			return nil, nil
		}

		outputLine := &OutputLine{}

		responseType := output.GetResponse()
		if responseType == nil {
			return outputLine, nil
		}

		//nolint:dupl // Similar structure to init response handling, but different protobuf types
		switch resp := responseType.(type) {
		case *proto.ShutdownResponse_Stdout:
			if resp.Stdout != nil {
				outputLine.Stdout = resp.Stdout.GetContent()
			}
		case *proto.ShutdownResponse_Stderr:
			if resp.Stderr != nil {
				outputLine.Stderr = resp.Stderr.GetContent()
			}
		case *proto.ShutdownResponse_ExitResult:
			if resp.ExitResult != nil {
				exitCode := int(resp.ExitResult.GetCode())
				if exitCode != 0 {
					l.Errorf("Engine shutdown failed with exit code %d", exitCode)
					return nil, errors.Errorf("%w with exit code %d", ErrEngineShutdownFailed, exitCode)
				}
			}
		case *proto.ShutdownResponse_Log:
			if resp.Log != nil {
				if logContent := resp.Log.GetContent(); logContent != "" {
					logEngineMessage(l, resp.Log.GetLevel(), logContent)
				}
			}
		}

		return outputLine, nil
	})
}

// OutputLine represents the output from the engine
type OutputLine struct {
	Stdout string
	Stderr string
}

type outputFn func() (*OutputLine, error)

// ReadEngineOutput reads the output from the engine, since grpc plugins don't have common type,
// use lambda function to read bytes from the stream
func ReadEngineOutput(runOptions *ExecutionOptions, forceStdErr bool, output outputFn) error {
	cmdStdout := runOptions.CmdStdout
	cmdStderr := runOptions.CmdStderr

	for {
		response, err := output()
		if err != nil && (errors.Is(err, ErrEngineInitFailed) || errors.Is(err, ErrEngineShutdownFailed)) {
			return err
		}

		if response == nil || err != nil {
			break
		}

		if response.Stdout != "" {
			if forceStdErr { // redirect stdout to stderr
				if _, err := cmdStderr.Write([]byte(response.Stdout)); err != nil {
					return errors.New(err)
				}
			} else {
				if _, err := cmdStdout.Write([]byte(response.Stdout)); err != nil {
					return errors.New(err)
				}
			}
		}

		if response.Stderr != "" {
			if _, err := cmdStderr.Write([]byte(response.Stderr)); err != nil {
				return errors.New(err)
			}
		}
	}
	// TODO: Why does this lint need to be ignored?
	return nil //nolint:nilerr
}

// ConvertMetaToProtobuf converts metadata map to protobuf map
func ConvertMetaToProtobuf(meta map[string]any) (map[string]*anypb.Any, error) {
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

// extract extracts a ZIP file into a specified destination directory.
func extract(l log.Logger, zipFile, destDir string) error {
	r, err := zip.OpenReader(zipFile)
	if err != nil {
		return errors.New(err)
	}

	defer func() {
		if closeErr := r.Close(); closeErr != nil {
			l.Warnf("warning: failed to close zip reader: %v", closeErr)
		}
	}()

	if err = os.MkdirAll(destDir, dirPerm); err != nil {
		return errors.New(err)
	}

	// Extract each file in the archive
	for _, file := range r.File {
		fPath := filepath.Join(destDir, file.Name)

		// Check for ZipSlip vulnerability
		if !strings.HasPrefix(fPath, filepath.Clean(destDir)+string(os.PathSeparator)) {
			return errors.New(err)
		}

		if file.FileInfo().IsDir() {
			// Create directories
			if err := os.MkdirAll(fPath, file.Mode()); err != nil {
				return errors.New(err)
			}

			continue
		}

		if err := os.MkdirAll(filepath.Dir(fPath), dirPerm); err != nil {
			return errors.New(err)
		}

		outFile, err := os.OpenFile(fPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, file.Mode())
		if err != nil {
			return errors.New(err)
		}

		defer func() {
			if closeErr := outFile.Close(); closeErr != nil {
				l.Warnf("warning: failed to close zip reader: %v", closeErr)
			}
		}()

		rc, err := file.Open()
		if err != nil {
			return errors.New(err)
		}

		defer func() {
			if closeErr := rc.Close(); closeErr != nil {
				l.Warnf("warning: failed to close file reader: %v", closeErr)
			}
		}()

		// Write file content
		if _, err := io.Copy(outFile, rc); err != nil {
			return errors.New(err)
		}
	}

	return nil
}

// detectFileType determines the type of file based on its magic bytes.
func detectFileType(l log.Logger, filePath string) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", errors.New(err)
	}

	defer func() {
		if closeErr := file.Close(); closeErr != nil {
			l.Warnf("warning: failed to close file : %v", filePath)
		}
	}()

	const headerSize = 4 // 4 bytes are enough for common formats

	header := make([]byte, headerSize)

	if _, err := file.Read(header); err != nil {
		return "", errors.New(err)
	}

	switch {
	case bytes.HasPrefix(header, []byte("PK\x03\x04")):
		return "zip", nil
	case bytes.HasPrefix(header, []byte("\x1F\x8B")):
		return "gzip", nil
	case bytes.HasPrefix(header, []byte("ustar")):
		return "tar", nil
	default:
		return "", nil
	}
}
