// Package engine provides the pluggable IaC engine for Terragrunt.
package engine

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/pkg/log/format/placeholders"
	logwriter "github.com/gruntwork-io/terragrunt/pkg/log/writer"

	"github.com/gruntwork-io/terragrunt/internal/cache"
	"github.com/gruntwork-io/terragrunt/internal/experiment"
	"github.com/gruntwork-io/terragrunt/internal/github"
	"github.com/gruntwork-io/terragrunt/internal/os/signal"
	"github.com/gruntwork-io/terragrunt/internal/telemetry"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/internal/vexec"
	"github.com/gruntwork-io/terragrunt/internal/vfs"

	"github.com/hashicorp/go-hclog"

	"google.golang.org/grpc/credentials/insecure"

	"errors"

	"github.com/gruntwork-io/terragrunt-engine-go/engine"
	"github.com/gruntwork-io/terragrunt-engine-go/proto"
	"github.com/gruntwork-io/terragrunt/internal/util"
	"github.com/gruntwork-io/terragrunt/internal/writer"
	"github.com/hashicorp/go-plugin"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/structpb"
)

const (
	engineVersion     = 1
	engineCookieKey   = "engine"
	engineCookieValue = "terragrunt"

	enginePluginsDir                            = "plugins"
	engineIACDir                                = "iac-engine"
	prefixTrim                                  = "terragrunt-"
	fileNameFormat                              = "terragrunt-iac-%s_%s_%s_%s_%s"
	checksumFileNameFormat                      = "terragrunt-iac-%s_%s_%s_SHA256SUMS"
	engineLogLevelEnv                           = "TG_ENGINE_LOG_LEVEL"
	defaultEngineRepoRoot                       = "github.com/"
	terraformCommandContextKey engineClientsKey = iota
	locksContextKey            engineLocksKey   = iota
	latestVersionsContextKey   engineLocksKey   = iota

	errMsgEngineClientsFetch = "failed to fetch engine clients from context"
	errMsgEngineClientsCast  = "failed to cast engine clients from context"
	errMsgVersionsCacheFetch = "failed to fetch engine versions cache from context"
	errMsgVersionsCacheCast  = "failed to cast engine versions cache from context"
)

const (
	// Cap extraction so malformed archives fail before they can exhaust disk space.
	engineArchiveDecompressedSizeLimit int64 = 512 << 20
	engineArchiveFilesLimit                  = 20
)

type (
	engineClientsKey byte
	engineLocksKey   byte
)

type ExecutionOptions struct {
	Writers           writer.Writers
	EngineOptions     *EngineOptions
	EngineConfig      *EngineConfig
	UnitDir           string
	CacheDir          string
	RootWorkingDir    string
	Command           string
	Args              []string
	Headless          bool
	ForwardTFStdout   bool
	SuppressStdout    bool
	AllocatePseudoTty bool

	LogShowAbsPaths        bool
	LogDisableErrorSummary bool
}

type engineInstance struct {
	engineClient *proto.EngineClient
	client       *plugin.Client
	execOptions  *ExecutionOptions
	// v carries the env the plugin was started with so Shutdown can address
	// the same environment long after Run returned.
	v venv.Venv
}

// engineEntry single-flights one cache dir's engine creation: the builder writes instance and
// err, then closes ready, so every other reader must receive on ready first. That close is
// their sole happens-before edge.
type engineEntry struct {
	ready       chan struct{}
	instance    *engineInstance
	err         error
	execOptions *ExecutionOptions
}

// engineClients is the registry of engine instances, keyed by post-download cache working dir.
// mu guards only map membership, never the slow create or shutdown work, so distinct cache
// dirs proceed in parallel. Every removal is under mu, so a batch Shutdown racing a per-unit
// ShutdownUnit never shuts an engine down twice.
type engineClients struct {
	clients map[string]*engineEntry
	mu      sync.Mutex
}

func newEngineClients() *engineClients {
	return &engineClients{clients: make(map[string]*engineEntry)}
}

// loadOrCreate returns the instance for key, or reserves the key and builds one via create.
// Concurrent callers for one key share a single build instead of racing to register competing
// engines. The bool reports whether an existing instance was reused.
func (c *engineClients) loadOrCreate(
	key string,
	execOptions *ExecutionOptions,
	create func() (*engineInstance, error),
) (*engineInstance, bool, error) {
	c.mu.Lock()

	if e, found := c.clients[key]; found {
		c.mu.Unlock()

		<-e.ready

		return e.instance, true, e.err
	}

	e := &engineEntry{ready: make(chan struct{}), execOptions: execOptions}
	c.clients[key] = e
	c.mu.Unlock()

	e.instance, e.err = create()

	if e.err != nil {
		// Drop a failed build so the next Run rebuilds instead of serving the cached failure.
		// The identity guard avoids deleting a different entry that replaced this one.
		c.mu.Lock()
		if c.clients[key] == e {
			delete(c.clients, key)
		}
		c.mu.Unlock()
	}

	close(e.ready)

	return e.instance, false, e.err
}

// takeAll removes and returns every registered entry.
func (c *engineClients) takeAll() []*engineEntry {
	c.mu.Lock()
	defer c.mu.Unlock()

	taken := make([]*engineEntry, 0, len(c.clients))
	for _, e := range c.clients {
		taken = append(taken, e)
	}

	clear(c.clients)

	return taken
}

// takeUnit removes and returns the entry for the given unit, or nil. It matches the
// entry's execOptions, not the instance's, so it finds a unit whose engine is still building.
func (c *engineClients) takeUnit(unitDir string) *engineEntry {
	c.mu.Lock()
	defer c.mu.Unlock()

	for key, e := range c.clients {
		if e.execOptions.UnitDir == unitDir {
			delete(c.clients, key)

			return e
		}
	}

	return nil
}

// Run executes the given command with the experimental engine. The executor
// on v spawns the engine plugin subprocess and must be OS-backed; v's env is
// forwarded to the plugin.
func Run(
	ctx context.Context,
	l log.Logger,
	v venv.Venv,
	execOptions *ExecutionOptions,
) (*util.CmdOutput, error) {
	engineClients, err := engineClientsFromContext(ctx)
	if err != nil {
		return nil, err
	}

	cacheDir := execOptions.CacheDir

	instance, found, err := engineClients.loadOrCreate(cacheDir, execOptions, func() (*engineInstance, error) {
		return createInstance(ctx, l, v, execOptions)
	})
	if err != nil {
		return nil, err
	}

	var output *util.CmdOutput

	runErr := telemetry.TelemeterFromContext(ctx).Collect(ctx, "engine_run", map[string]any{
		"command":            execOptions.Command,
		"cache_dir":          cacheDir,
		"engine_initialized": found,
	}, func(runCtx context.Context) error {
		var invokeErr error

		output, invokeErr = invoke(runCtx, l, v, execOptions, instance.engineClient)

		return invokeErr
	})
	if runErr != nil {
		return nil, runErr
	}

	return output, nil
}

// createInstance downloads, starts, and initializes an engine for execOptions. A plugin
// that starts but fails to initialize is killed rather than leaked, since it is never
// registered for a later Shutdown to reach.
func createInstance(
	ctx context.Context,
	l log.Logger,
	v venv.Venv,
	execOptions *ExecutionOptions,
) (*engineInstance, error) {
	if err := downloadEngine(ctx, l, execOptions); err != nil {
		return nil, err
	}

	terragruntEngine, client, err := createEngine(ctx, l, v.Exec, execOptions)
	if err != nil {
		return nil, err
	}

	if err := initialize(ctx, l, v, execOptions, terragruntEngine); err != nil {
		client.Kill()

		return nil, err
	}

	return &engineInstance{
		engineClient: terragruntEngine,
		client:       client,
		execOptions:  execOptions,
		v:            v,
	}, nil
}

// WithEngineValues add to context default values for engine.
func WithEngineValues(ctx context.Context) context.Context {
	ctx = context.WithValue(ctx, terraformCommandContextKey, newEngineClients())
	ctx = context.WithValue(ctx, locksContextKey, util.NewKeyLocks())
	ctx = context.WithValue(ctx, latestVersionsContextKey, cache.NewCache[string]("engineVersions"))

	return ctx
}

// downloadEngine downloads the engine for the given options.
func downloadEngine(ctx context.Context, l log.Logger, execOptions *ExecutionOptions) error {
	e := execOptions.EngineConfig
	if e == nil {
		return nil
	}

	if util.FileExists(e.Source) {
		// if source is a file, no need to download, exit
		return nil
	}

	return telemetry.TelemeterFromContext(ctx).Collect(ctx, "engine_download", map[string]any{
		"source":  e.Source,
		"version": e.Version,
	}, func(ctx context.Context) error {
		// If source is empty, we cannot download the engine
		// This indicates an engine block was configured but source was not provided
		if e.Source == "" {
			return errors.New(
				"engine block is configured but source is empty, please provide an engine source or remove the engine block",
			)
		}

		// identify engine version if not specified
		if len(e.Version) == 0 {
			if !isDirectURL(e.Source) {
				tag, err := lastReleaseVersion(ctx, execOptions)
				if err != nil {
					return err
				}

				e.Version = tag
			}
		}

		path, err := engineDir(execOptions)
		if err != nil {
			return err
		}

		if ensureErr := util.EnsureDirectory(path); ensureErr != nil {
			return ensureErr
		}

		localEngineFile := filepath.Join(path, engineFileName(e))

		// lock downloading process for only one instance
		locks, err := downloadLocksFromContext(ctx)
		if err != nil {
			return err
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
		if !isDirectURL(e.Source) {
			checksumFile = filepath.Join(path, engineChecksumName(e))
			checksumSigFile = filepath.Join(path, engineChecksumSigName(e))
			assets.ChecksumFile = checksumFile
			assets.ChecksumSigFile = checksumSigFile
		}

		// Create download client and download assets
		downloadClient := github.NewGitHubReleasesDownloadClient(github.WithLogger(l))

		result, err := downloadClient.DownloadReleaseAssets(ctx, assets)
		if err != nil {
			return fmt.Errorf("failed to download engine assets: %w", err)
		}

		// Update file paths from result
		downloadFile = result.PackageFile
		checksumFile = result.ChecksumFile
		checksumSigFile = result.ChecksumSigFile

		if !execOptions.EngineOptions.SkipChecksumCheck && checksumFile != "" && checksumSigFile != "" {
			l.Infof("Verifying checksum for %s", downloadFile)

			if err := verifyFile(downloadFile, checksumFile, checksumSigFile); err != nil {
				return err
			}
		} else {
			l.Warnf("Skipping verification for %s", downloadFile)
		}

		if err := extractArchive(l, downloadFile, localEngineFile); err != nil {
			return err
		}

		l.Infof("Engine available as %s", path)

		return nil
	})
}

func lastReleaseVersion(ctx context.Context, opts *ExecutionOptions) (string, error) {
	repository := strings.TrimPrefix(opts.EngineConfig.Source, defaultEngineRepoRoot)

	versionCache, err := engineVersionsCacheFromContext(ctx)
	if err != nil {
		return "", err
	}

	cacheKey := "github_release_" + repository
	if val, found := versionCache.Get(ctx, cacheKey); found {
		return val, nil
	}

	githubClient := github.NewGitHubAPIClient(github.WithGithubComDefaultAuth())

	tag, err := githubClient.GetLatestReleaseTag(ctx, repository)
	if err != nil {
		return "", fmt.Errorf("failed to get latest release for repository %s: %w", repository, err)
	}

	versionCache.Put(ctx, cacheKey, tag)

	return tag, nil
}

func extractArchive(l log.Logger, downloadFile string, engineFile string) error {
	return extractArchiveWithLimits(
		l,
		downloadFile,
		engineFile,
		engineArchiveDecompressedSizeLimit,
		engineArchiveFilesLimit,
	)
}

func extractArchiveWithLimits(
	l log.Logger,
	downloadFile string,
	engineFile string,
	decompressedSizeLimit int64,
	filesLimit int,
) error {
	if !isArchiveByHeader(l, downloadFile) {
		l.Info("Downloaded file is not an archive, no extraction needed")
		// move file directly if it is not an archive
		if err := os.Rename(downloadFile, engineFile); err != nil {
			return err
		}

		return nil
	}
	// extract package and process files
	path := filepath.Dir(engineFile)

	tempDir, err := os.MkdirTemp(path, "temp-")
	if err != nil {
		return err
	}

	defer func() {
		if err = os.RemoveAll(tempDir); err != nil {
			l.Warnf("Failed to clean temp dir %s: %v", tempDir, err)
		}
	}()
	// extract archive
	if err = vfs.NewZipDecompressor(
		vfs.WithFileSizeLimit(decompressedSizeLimit),
		vfs.WithFilesLimit(filesLimit),
	).Unzip(l, vfs.NewOSFS(), tempDir, downloadFile, 0); err != nil {
		return newArchiveExtractionError(downloadFile, err)
	}

	// process files
	files, err := os.ReadDir(tempDir)
	if err != nil {
		return err
	}

	l.Infof("Engine extracted to %s", path)

	if len(files) == 1 && !files[0].IsDir() {
		// handle case where archive contains a single file, most of the cases
		singleFile := filepath.Join(tempDir, files[0].Name())
		if err := os.Rename(singleFile, engineFile); err != nil {
			return err
		}

		return nil
	}

	// Move all files to the engine directory
	for _, file := range files {
		srcPath := filepath.Join(tempDir, file.Name())

		dstPath := filepath.Join(path, file.Name())
		if err := os.Rename(srcPath, dstPath); err != nil {
			return err
		}
	}

	return nil
}

// engineDir returns the directory path where engine files are stored.
func engineDir(opts *ExecutionOptions) (string, error) {
	engine := opts.EngineConfig
	if util.FileExists(engine.Source) {
		return filepath.Dir(engine.Source), nil
	}

	platform := runtime.GOOS
	arch := runtime.GOARCH

	if cacheDir := opts.EngineOptions.CachePath; len(cacheDir) != 0 {
		return filepath.Join(
			cacheDir,
			"terragrunt",
			enginePluginsDir,
			engineIACDir,
			engine.Type,
			engine.Version,
			platform,
			arch,
		), nil
	}

	cacheDir, err := util.EnsureCacheDir()
	if err != nil {
		return "", err
	}

	return filepath.Join(
		cacheDir,
		enginePluginsDir,
		engineIACDir,
		engine.Type,
		engine.Version,
		platform,
		arch,
	), nil
}

// engineFileName returns the file name for the engine.
func engineFileName(e *EngineConfig) string {
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
func engineChecksumName(e *EngineConfig) string {
	engineName := filepath.Base(e.Source)

	engineName = strings.TrimPrefix(engineName, prefixTrim)

	return fmt.Sprintf(checksumFileNameFormat, engineName, e.Type, e.Version)
}

// engineChecksumSigName returns the file name of engine checksum file signature
func engineChecksumSigName(e *EngineConfig) string {
	return engineChecksumName(e) + ".sig"
}

// enginePackageName returns the package name for the engine.
func enginePackageName(e *EngineConfig) string {
	return engineFileName(e) + ".zip"
}

func isDirectURL(source string) bool {
	return strings.Contains(source, "://")
}

// isArchiveByHeader checks if a file is an archive by examining its header.
func isArchiveByHeader(l log.Logger, filePath string) bool {
	archiveType, err := detectFileType(l, filePath)

	return err == nil && archiveType != ""
}

// engineClientsFromContext returns the engine clients registry from the context.
func engineClientsFromContext(ctx context.Context) (*engineClients, error) {
	val := ctx.Value(terraformCommandContextKey)
	if val == nil {
		return nil, errors.New(errMsgEngineClientsFetch)
	}

	result, ok := val.(*engineClients)
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
	// gracefulExitTimeout is the grace period for a plugin to exit on its own.
	gracefulExitTimeout = 5 * time.Second
	// pluginExitPollInterval is the cadence for polling whether the plugin has exited.
	pluginExitPollInterval = 50 * time.Millisecond
	// shutdownRPCTimeout bounds the Shutdown RPC stream.
	shutdownRPCTimeout = 30 * time.Second
)

// Shutdown shuts down the experimental engine.
func Shutdown(ctx context.Context, l log.Logger, experiments experiment.Experiments, noEngine bool) error {
	if !experiments.Evaluate(experiment.IacEngine) || noEngine {
		return nil
	}

	engineClients, err := engineClientsFromContext(ctx)
	if err != nil {
		return err
	}

	for _, entry := range engineClients.takeAll() {
		drainEntry(ctx, l, entry)
	}

	return nil
}

// ShutdownUnit shuts down and removes the engine bound to a single unit, releasing its
// plugin subprocess when the unit finishes rather than at the batch Shutdown. It is
// experiment-gated; the only error it returns is a missing engine-clients context.
func ShutdownUnit(
	ctx context.Context,
	l log.Logger,
	experiments experiment.Experiments,
	noEngine bool,
	unitDir string,
) error {
	if !experiments.Evaluate(experiment.IacEngine) || noEngine {
		return nil
	}

	engineClients, err := engineClientsFromContext(ctx)
	if err != nil {
		return err
	}

	if entry := engineClients.takeUnit(filepath.Clean(unitDir)); entry != nil {
		drainEntry(ctx, l, entry)
	}

	return nil
}

// drainEntry shuts down the entry's engine after its build settles. A failed build leaves a
// nil instance and nothing to release: createInstance already killed any half-built plugin.
func drainEntry(ctx context.Context, l log.Logger, e *engineEntry) {
	<-e.ready

	if e.instance == nil {
		return
	}

	shutdownInstance(ctx, l, e.instance)
}

// shutdownInstance tears down one engine, force-killing a plugin that will not exit in time.
// Errors are logged, not returned: shutdown is best-effort and must not fail the run. The
// context is detached from cancellation so an already-cancelled run still shuts engines down,
// then bounded by shutdownRPCTimeout so a hung Shutdown stream falls through to the force-kill
// below instead of blocking the worker.
func shutdownInstance(ctx context.Context, l log.Logger, instance *engineInstance) {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), shutdownRPCTimeout)
	defer cancel()

	l.Debugf("Shutting down engine for %s", instance.execOptions.CacheDir)

	if err := shutdown(ctx, l, instance.v, instance.execOptions, instance.engineClient); err != nil {
		l.Errorf("Error shutting down engine: %v", err)
	}

	// The shutdown RPC already told the plugin to exit, so wait before force-killing it.
	if !WaitForPluginExit(instance.client.Exited, gracefulExitTimeout) {
		l.Debugf("Plugin did not exit gracefully within timeout, force killing")
		instance.client.Kill()
	}
}

// WaitForPluginExit reports whether the plugin exited within the timeout, polling at
// pluginExitPollInterval.
func WaitForPluginExit(exited func() bool, timeout time.Duration) bool {
	deadline := time.After(timeout)

	ticker := time.NewTicker(pluginExitPollInterval)
	defer ticker.Stop()

	for {
		if exited() {
			return true
		}

		select {
		case <-deadline:
			return false
		case <-ticker.C:
		}
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
	e vexec.Exec,
	execOptions *ExecutionOptions,
) (*proto.EngineClient, *plugin.Client, error) {
	if execOptions.EngineConfig == nil {
		return nil, nil, errors.New("engine options are nil")
	}

	// If source is empty, we cannot determine the engine file path
	if execOptions.EngineConfig.Source == "" {
		return nil, nil, errors.New("engine source is empty, cannot create engine")
	}

	var (
		engineClient *proto.EngineClient
		pluginClient *plugin.Client
	)

	err := telemetry.TelemeterFromContext(ctx).Collect(ctx, "engine_create", map[string]any{
		"source":    execOptions.EngineConfig.Source,
		"version":   execOptions.EngineConfig.Version,
		"cache_dir": execOptions.CacheDir,
	}, func(ctx context.Context) error {
		path, err := engineDir(execOptions)
		if err != nil {
			return err
		}

		localEnginePath := filepath.Join(path, engineFileName(execOptions.EngineConfig))
		localChecksumFile := filepath.Join(path, engineChecksumName(execOptions.EngineConfig))
		localChecksumSigFile := filepath.Join(path, engineChecksumSigName(execOptions.EngineConfig))

		// validate engine before loading if verification is not disabled
		skipCheck := execOptions.EngineOptions.SkipChecksumCheck
		if !skipCheck && util.FileExists(localEnginePath) && util.FileExists(localChecksumFile) &&
			util.FileExists(localChecksumSigFile) {
			if err = verifyFile(localEnginePath, localChecksumFile, localChecksumSigFile); err != nil {
				return err
			}
		} else {
			l.Warnf("Skipping verification for %s", localEnginePath)
		}

		l.Debugf("Creating engine %s", localEnginePath)

		engineLogLevel := execOptions.EngineOptions.LogLevel

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
		cmd := e.Command(context.WithoutCancel(ctx), localEnginePath)
		cmd.SetEnv([]string{fmt.Sprintf("%s=%s", engineLogLevelEnv, engineLogLevel)})
		cmd.SetCancel(func() error {
			sig := signal.SignalFromContext(ctx)
			if sig == nil {
				sig = os.Kill
			}

			if err := cmd.Signal(sig); err != nil && !errors.Is(err, vexec.ErrProcessNotStarted) {
				return err
			}

			return nil
		})

		// hashicorp/go-plugin's ClientConfig requires a concrete *exec.Cmd.
		osCmder, ok := cmd.(vexec.OSCmder)
		if !ok {
			return fmt.Errorf("engine plugin spawn: %w", vexec.ErrNotOSBacked)
		}

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
			Cmd: osCmder.OSCmd(),
			GRPCDialOptions: []grpc.DialOption{
				grpc.WithTransportCredentials(insecure.NewCredentials()),
			},
			AllowedProtocols: []plugin.Protocol{plugin.ProtocolGRPC},
		})

		rpcClient, err := client.Client()
		if err != nil {
			return err
		}

		rawClient, err := rpcClient.Dispense("plugin")
		if err != nil {
			return err
		}

		terragruntEngine, ok := rawClient.(proto.EngineClient)
		if !ok {
			return fmt.Errorf("engine plugin returned unexpected client type %T", rawClient)
		}

		engineClient = &terragruntEngine
		pluginClient = client

		return nil
	})
	if err != nil {
		return nil, nil, err
	}

	return engineClient, pluginClient, nil
}

// invoke engine for working directory
func invoke(
	ctx context.Context,
	l log.Logger,
	v venv.Venv,
	runOptions *ExecutionOptions,
	client *proto.EngineClient,
) (*util.CmdOutput, error) {
	var result *util.CmdOutput

	err := telemetry.TelemeterFromContext(ctx).Collect(ctx, "engine_invoke", map[string]any{
		"command":   runOptions.Command,
		"cache_dir": runOptions.CacheDir,
	}, func(ctx context.Context) error {
		l = l.WithField(placeholders.TFPathKeyName, "engine")

		meta, err := ConvertMetaToProtobuf(runOptions.EngineConfig.Meta)
		if err != nil {
			return err
		}

		response, err := (*client).Run(ctx, &proto.RunRequest{
			Command:           runOptions.Command,
			Args:              runOptions.Args,
			AllocatePseudoTty: runOptions.AllocatePseudoTty,
			WorkingDir:        runOptions.CacheDir,
			Meta:              meta,
			EnvVars:           v.Env,
		})
		if err != nil {
			return err
		}

		// Determine log levels based on headless mode (similar to buildOutWriter/buildErrWriter)
		stdoutLogLevel := log.StdoutLevel
		stderrLogLevel := log.StderrLevel

		stdoutWriter := writer.ExtractOriginalWriter(runOptions.Writers.Writer)
		stderrWriter := writer.ExtractOriginalWriter(runOptions.Writers.ErrWriter)

		if runOptions.Headless && !runOptions.ForwardTFStdout {
			stdoutLogLevel = log.InfoLevel
			stderrLogLevel = log.ErrorLevel
			stdoutWriter = writer.ExtractOriginalWriter(runOptions.Writers.ErrWriter)
		}

		var (
			output = util.CmdOutput{}

			// Use the original output writers (before they were wrapped by logTFOutput)
			// and create new writers with the engine logger
			engineStdout = logwriter.New(
				logwriter.WithLogger(l.WithOptions(log.WithOutput(stdoutWriter))),
				logwriter.WithDefaultLevel(stdoutLogLevel),
				logwriter.WithMsgSeparator("\n"),
			)
			engineStderr = logwriter.New(
				logwriter.WithLogger(l.WithOptions(log.WithOutput(stderrWriter))),
				logwriter.WithDefaultLevel(stderrLogLevel),
				logwriter.WithMsgSeparator("\n"),
			)

			stdout = io.MultiWriter(engineStdout, &output.Stdout)
			stderr = io.MultiWriter(engineStderr, &output.Stderr)
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
						return err
					}
				}
			case *proto.RunResponse_Stderr:
				if resp.Stderr != nil {
					if err = processStream(resp.Stderr.GetContent(), &stderrLineBuf, stderr); err != nil {
						return err
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
			return err
		}

		if err = flushBuffer(&stderrLineBuf, stderr); err != nil {
			return err
		}

		l.Debugf("Engine execution done in %v", runOptions.CacheDir)

		if resultCode != 0 {
			err = util.ProcessExecutionError{
				Err:             fmt.Errorf("command failed with exit code %d", resultCode),
				Output:          output,
				WorkingDir:      runOptions.CacheDir,
				RootWorkingDir:  runOptions.RootWorkingDir,
				LogShowAbsPaths: runOptions.LogShowAbsPaths,
				Command:         runOptions.Command,
				Args:            runOptions.Args,
				DisableSummary:  runOptions.LogDisableErrorSummary,
			}

			return err
		}

		result = &output

		return nil
	})
	if err != nil {
		return nil, err
	}

	return result, nil
}

// processStream handles the character buffering and line printing for a given stream
func processStream(data string, lineBuf *bytes.Buffer, output io.Writer) error {
	for _, ch := range data {
		lineBuf.WriteRune(ch)

		if ch == '\n' {
			if _, err := fmt.Fprint(output, lineBuf.String()); err != nil {
				return err
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
			return err
		}
	}

	return nil
}

var ErrEngineInitFailed = errors.New("engine init failed")

// initialize engine for working directory
func initialize(
	ctx context.Context,
	l log.Logger,
	v venv.Venv,
	runOptions *ExecutionOptions,
	client *proto.EngineClient,
) error {
	return telemetry.TelemeterFromContext(ctx).Collect(ctx, "engine_initialize", map[string]any{
		"cache_dir": runOptions.CacheDir,
	}, func(ctx context.Context) error {
		meta, err := ConvertMetaToProtobuf(runOptions.EngineConfig.Meta)
		if err != nil {
			return err
		}

		l.Debugf("Running init for engine in %s", runOptions.CacheDir)

		request, err := (*client).Init(ctx, &proto.InitRequest{
			EnvVars:    v.Env,
			WorkingDir: runOptions.CacheDir,
			Meta:       meta,
		})
		if err != nil {
			return err
		}

		l.Debugf("Reading init output for engine in %s", runOptions.CacheDir)

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
						return nil, fmt.Errorf("%w with exit code %d", ErrEngineInitFailed, exitCode)
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
	})
}

var ErrEngineShutdownFailed = errors.New("engine shutdown failed")

// shutdown engine for working directory
func shutdown(
	ctx context.Context,
	l log.Logger,
	v venv.Venv,
	runOptions *ExecutionOptions,
	terragruntEngine *proto.EngineClient,
) error {
	return telemetry.TelemeterFromContext(ctx).Collect(ctx, "engine_shutdown", map[string]any{
		"cache_dir": runOptions.CacheDir,
	}, func(ctx context.Context) error {
		meta, err := ConvertMetaToProtobuf(runOptions.EngineConfig.Meta)
		if err != nil {
			return err
		}

		request, err := (*terragruntEngine).Shutdown(ctx, &proto.ShutdownRequest{
			WorkingDir: runOptions.CacheDir,
			Meta:       meta,
			EnvVars:    v.Env,
		})
		if err != nil {
			return err
		}

		l.Debugf("Reading shutdown output for engine in %s", runOptions.CacheDir)

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
						return nil, fmt.Errorf("%w with exit code %d", ErrEngineShutdownFailed, exitCode)
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
	cmdStdout := runOptions.Writers.Writer
	cmdStderr := runOptions.Writers.ErrWriter

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
					return err
				}
			} else {
				if _, err := cmdStdout.Write([]byte(response.Stdout)); err != nil {
					return err
				}
			}
		}

		if response.Stderr != "" {
			if _, err := cmdStderr.Write([]byte(response.Stderr)); err != nil {
				return err
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

// detectFileType determines the type of file based on its magic bytes.
func detectFileType(l log.Logger, filePath string) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", err
	}

	defer func() {
		if closeErr := file.Close(); closeErr != nil {
			l.Warnf("warning: failed to close file : %v", filePath)
		}
	}()

	const headerSize = 4 // 4 bytes are enough for common formats

	header := make([]byte, headerSize)

	if _, err := file.Read(header); err != nil {
		return "", err
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
