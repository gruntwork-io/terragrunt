// Package engine provides the pluggable IaC engine for Terragrunt.
package engine

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"github.com/gruntwork-io/terragrunt/internal/cache"

	"github.com/hashicorp/go-getter"

	"github.com/hashicorp/go-hclog"

	"google.golang.org/grpc/credentials/insecure"

	"github.com/gruntwork-io/terragrunt-engine-go/engine"
	"github.com/gruntwork-io/terragrunt-engine-go/proto"
	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/util"
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
		return nil, errors.New(err)
	}

	workingDir := runOptions.TerragruntOptions.WorkingDir
	instance, found := engineClients.Load(workingDir)
	// initialize engine for working directory
	if !found {
		// download engine if not available
		if err = DownloadEngine(ctx, runOptions.TerragruntOptions); err != nil {
			return nil, errors.New(err)
		}

		terragruntEngine, client, err := createEngine(runOptions.TerragruntOptions)
		if err != nil {
			return nil, errors.New(err)
		}

		engineClients.Store(workingDir, &engineInstance{
			terragruntEngine: terragruntEngine,
			client:           client,
			executionOptions: runOptions,
		})

		instance, _ = engineClients.Load(workingDir)

		if err := initialize(ctx, runOptions, terragruntEngine); err != nil {
			return nil, errors.New(err)
		}
	}

	engInst, ok := instance.(*engineInstance)
	if !ok {
		return nil, errors.Errorf("failed to fetch engine instance %s", workingDir)
	}

	terragruntEngine := engInst.terragruntEngine

	output, err := invoke(ctx, runOptions, terragruntEngine)
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
func DownloadEngine(ctx context.Context, opts *options.TerragruntOptions) error {
	if !opts.EngineEnabled {
		return nil
	}

	e := opts.Engine

	if util.FileExists(e.Source) {
		// if source is a file, no need to download, exit
		return nil
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

	if err := util.EnsureDirectory(path); err != nil {
		return errors.New(err)
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
			return errors.New(err)
		}
	}

	if !opts.EngineSkipChecksumCheck && checksumFile != "" && checksumSigFile != "" {
		opts.Logger.Infof("Verifying checksum for %s", downloadFile)

		if err := verifyFile(downloadFile, checksumFile, checksumSigFile); err != nil {
			return errors.New(err)
		}
	} else {
		opts.Logger.Warnf("Skipping verification for %s", downloadFile)
	}

	if err := extractArchive(opts, downloadFile, localEngineFile); err != nil {
		return errors.New(err)
	}

	opts.Logger.Infof("Engine available as %s", path)

	return nil
}

func lastReleaseVersion(ctx context.Context, opts *options.TerragruntOptions) (string, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/releases/latest", strings.TrimPrefix(opts.Engine.Source, defaultEngineRepoRoot))

	versionCache, err := engineVersionsCacheFromContext(ctx)

	if err != nil {
		return "", errors.New(err)
	}

	if val, found := versionCache.Get(ctx, url); found {
		return val, nil
	}

	type release struct {
		Tag string `json:"tag_name"`
	}
	// query tag from https://api.github.com/repos/{owner}/{repo}/releases/latest
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)

	if err != nil {
		return "", errors.New(err)
	}

	client := &http.Client{}
	resp, err := client.Do(req)

	if err != nil {
		return "", errors.New(err)
	}

	defer resp.Body.Close() //nolint:errcheck
	body, err := io.ReadAll(resp.Body)

	if err != nil {
		return "", errors.New(err)
	}

	var r release
	if err := json.Unmarshal(body, &r); err != nil {
		return "", errors.New(err)
	}

	versionCache.Put(ctx, url, r.Tag)

	return r.Tag, nil
}

func extractArchive(opts *options.TerragruntOptions, downloadFile string, engineFile string) error {
	if !isArchiveByHeader(opts, downloadFile) {
		opts.Logger.Info("Downloaded file is not an archive, no extraction needed")
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
		if err := os.RemoveAll(tempDir); err != nil {
			opts.Logger.Warnf("Failed to clean temp dir %s: %v", tempDir, err)
		}
	}()
	// extract archive
	if err := extract(opts, downloadFile, tempDir); err != nil {
		return errors.New(err)
	}
	// process files
	files, err := os.ReadDir(tempDir)
	if err != nil {
		return errors.New(err)
	}

	opts.Logger.Infof("Engine extracted to %s", path)

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
func isArchiveByHeader(opts *options.TerragruntOptions, filePath string) bool {
	archiveType, err := detectFileType(opts, filePath)

	return err == nil && archiveType != ""
}

// engineClientsFromContext returns the engine clients map from the context.
func engineClientsFromContext(ctx context.Context) (*sync.Map, error) {
	val := ctx.Value(terraformCommandContextKey)
	if val == nil {
		return nil, errors.New(errors.New("failed to fetch engine clients from context"))
	}

	result, ok := val.(*sync.Map)
	if !ok {
		return nil, errors.New(errors.New("failed to cast engine clients from context"))
	}

	return result, nil
}

// downloadLocksFromContext returns the locks map from the context.
func downloadLocksFromContext(ctx context.Context) (*util.KeyLocks, error) {
	val := ctx.Value(locksContextKey)
	if val == nil {
		return nil, errors.New(errors.New("failed to fetch engine clients from context"))
	}

	result, ok := val.(*util.KeyLocks)
	if !ok {
		return nil, errors.New(errors.New("failed to cast engine clients from context"))
	}

	return result, nil
}

func engineVersionsCacheFromContext(ctx context.Context) (*cache.Cache[string], error) {
	val := ctx.Value(latestVersionsContextKey)
	if val == nil {
		return nil, errors.New(errors.New("failed to fetch engine versions cache from context"))
	}

	result, ok := val.(*cache.Cache[string])
	if !ok {
		return nil, errors.New(errors.New("failed to cast engine versions cache from context"))
	}

	return result, nil
}

// Shutdown shuts down the experimental engine.
func Shutdown(ctx context.Context, opts *options.TerragruntOptions) error {
	if !opts.EngineEnabled {
		return nil
	}

	// iterate over all engine instances and shutdown
	engineClients, err := engineClientsFromContext(ctx)
	if err != nil {
		return errors.New(err)
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
	path, err := engineDir(terragruntOptions)
	if err != nil {
		return nil, nil, errors.New(err)
	}

	localEnginePath := filepath.Join(path, engineFileName(terragruntOptions.Engine))
	localChecksumFile := filepath.Join(path, engineChecksumName(terragruntOptions.Engine))
	localChecksumSigFile := filepath.Join(path, engineChecksumSigName(terragruntOptions.Engine))

	// validate engine before loading if verification is not disabled
	skipCheck := terragruntOptions.EngineSkipChecksumCheck
	if !skipCheck && util.FileExists(localEnginePath) && util.FileExists(localChecksumFile) &&
		util.FileExists(localChecksumSigFile) {
		if err := verifyFile(localEnginePath, localChecksumFile, localChecksumSigFile); err != nil {
			return nil, nil, errors.New(err)
		}
	} else {
		terragruntOptions.Logger.Warnf("Skipping verification for %s", localEnginePath)
	}

	terragruntOptions.Logger.Debugf("Creating engine %s", localEnginePath)

	engineLogLevel := terragruntOptions.EngineLogLevel
	if len(engineLogLevel) == 0 {
		engineLogLevel = terragruntOptions.LogLevel.String()
		// turn off log formatting if disabled for Terragrunt
		if terragruntOptions.DisableLog {
			engineLogLevel = hclog.Off.String()
		}
	}

	logger := hclog.NewInterceptLogger(&hclog.LoggerOptions{
		Level:  hclog.LevelFromString(engineLogLevel),
		Output: terragruntOptions.Logger.Writer(),
	})

	cmd := exec.Command(localEnginePath)
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
func invoke(ctx context.Context, runOptions *ExecutionOptions, client *proto.EngineClient) (*util.CmdOutput, error) {
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
		runResp, err := response.Recv()
		if err != nil || runResp == nil {
			break
		}

		if err := processStream(runResp.GetStdout(), &stdoutLineBuf, stdout); err != nil {
			return nil, errors.New(err)
		}

		if err := processStream(runResp.GetStderr(), &stderrLineBuf, stderr); err != nil {
			return nil, errors.New(err)
		}

		resultCode = int(runResp.GetResultCode())
	}

	if err := flushBuffer(&stdoutLineBuf, stdout); err != nil {
		return nil, errors.New(err)
	}

	if err := flushBuffer(&stderrLineBuf, stderr); err != nil {
		return nil, errors.New(err)
	}

	opts.Logger.Debugf("Engine execution done in %v", opts.WorkingDir)

	if resultCode != 0 {
		err = util.ProcessExecutionError{
			Err:            errors.Errorf("command failed with exit code %d", resultCode),
			Output:         output,
			WorkingDir:     opts.WorkingDir,
			Command:        runOptions.Command,
			Args:           runOptions.Args,
			DisableSummary: opts.LogDisableErrorSummary,
		}

		return nil, errors.New(err)
	}

	return &output, nil
}

// processStream handles the character buffering and line printing for a given stream
func processStream(data string, lineBuf *bytes.Buffer, output io.Writer) error {
	for _, ch := range data {
		lineBuf.WriteByte(byte(ch))

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

// initialize engine for working directory
func initialize(ctx context.Context, runOptions *ExecutionOptions, client *proto.EngineClient) error {
	terragruntOptions := runOptions.TerragruntOptions

	meta, err := ConvertMetaToProtobuf(runOptions.TerragruntOptions.Engine.Meta)
	if err != nil {
		return errors.New(err)
	}

	terragruntOptions.Logger.Debugf("Running init for engine in %s", runOptions.WorkingDir)

	request, err := (*client).Init(ctx, &proto.InitRequest{
		EnvVars:    runOptions.TerragruntOptions.Env,
		WorkingDir: runOptions.WorkingDir,
		Meta:       meta,
	})
	if err != nil {
		return errors.New(err)
	}

	terragruntOptions.Logger.Debugf("Reading init output for engine in %s", runOptions.WorkingDir)

	return ReadEngineOutput(runOptions, true, func() (*OutputLine, error) {
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

	terragruntOptions.Logger.Debugf("Reading shutdown output for engine in %s", runOptions.WorkingDir)

	return ReadEngineOutput(runOptions, true, func() (*OutputLine, error) {
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

// extract extracts a ZIP file into a specified destination directory.
func extract(opts *options.TerragruntOptions, zipFile, destDir string) error {
	r, err := zip.OpenReader(zipFile)
	if err != nil {
		return errors.New(err)
	}

	defer func() {
		if closeErr := r.Close(); closeErr != nil {
			opts.Logger.Warnf("warning: failed to close zip reader: %v", closeErr)
		}
	}()

	const dirPerm = 0755
	if err := os.MkdirAll(destDir, dirPerm); err != nil {
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

		const dirPerm = 0755
		if err := os.MkdirAll(filepath.Dir(fPath), dirPerm); err != nil {
			return errors.New(err)
		}

		outFile, err := os.OpenFile(fPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, file.Mode())
		if err != nil {
			return errors.New(err)
		}

		defer func() {
			if closeErr := outFile.Close(); closeErr != nil {
				opts.Logger.Warnf("warning: failed to close zip reader: %v", closeErr)
			}
		}()

		rc, err := file.Open()
		if err != nil {
			return errors.New(err)
		}

		defer func() {
			if closeErr := rc.Close(); closeErr != nil {
				opts.Logger.Warnf("warning: failed to close file reader: %v", closeErr)
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
func detectFileType(opts *options.TerragruntOptions, filePath string) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", errors.New(err)
	}

	defer func() {
		if closeErr := file.Close(); closeErr != nil {
			opts.Logger.Warnf("warning: failed to close file : %v", filePath)
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
