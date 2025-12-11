// Package options provides a set of options that configure the behavior of the Terragrunt program.
package options

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/gruntwork-io/terragrunt/internal/cli"
	"github.com/gruntwork-io/terragrunt/internal/cloner"
	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/internal/experiment"
	"github.com/gruntwork-io/terragrunt/internal/report"
	"github.com/gruntwork-io/terragrunt/internal/strict"
	"github.com/gruntwork-io/terragrunt/internal/strict/controls"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/pkg/log/format"
	"github.com/gruntwork-io/terragrunt/pkg/log/format/placeholders"
	"github.com/gruntwork-io/terragrunt/telemetry"
	"github.com/gruntwork-io/terragrunt/util"
	"github.com/hashicorp/go-version"
	"github.com/puzpuzpuz/xsync/v3"
)

const ContextKey ctxKey = iota

const (
	DefaultMaxFoldersToCheck = 100

	// no limits on parallelism by default (limited by GOPROCS)
	DefaultParallelism = math.MaxInt32

	// TofuDefaultPath command to run tofu
	TofuDefaultPath = "tofu"

	// TerraformDefaultPath just takes terraform from the path
	TerraformDefaultPath = "terraform"

	// Default to naming it `terragrunt_rendered.json` in the terragrunt config directory.
	DefaultJSONOutName = "terragrunt_rendered.json"

	DefaultSignalsFile = "error-signals.json"

	DefaultTFDataDir = ".terraform"

	DefaultIAMAssumeRoleDuration = 3600

	minCommandLength = 2

	defaultExcludesFile = ".terragrunt-excludes"

	DefaultLogLevel = log.InfoLevel
)

var (
	DefaultWrappedPath = identifyDefaultWrappedExecutable(context.Background())

	defaultProviderCacheRegistryNames = []string{
		"registry.terraform.io",
		"registry.opentofu.org",
	}

	TerraformCommandsWithSubcommand = []string{
		"debug",
		"force-unlock",
		"state",
	}

	defaultVersionManagerFileName = []string{
		".terraform-version",
		".tool-versions",
		"mise.toml",
		".mise.toml",
	}

	// Pattern used to clean error message when looking for retry and ignore patterns.
	errorCleanPattern = regexp.MustCompile(`[^a-zA-Z0-9./'"(): ]+`)
)

type ctxKey byte

type TerraformImplementationType string

const (
	TerraformImpl TerraformImplementationType = "terraform"
	OpenTofuImpl  TerraformImplementationType = "tofu"
	UnknownImpl   TerraformImplementationType = "unknown"
)

// TerragruntOptions represents options that configure the behavior of the Terragrunt program
type TerragruntOptions struct {
	// If you want stdout to go somewhere other than os.stdout
	Writer io.Writer
	// If you want stderr to go somewhere other than os.stderr
	ErrWriter io.Writer
	// Version of terragrunt
	TerragruntVersion *version.Version `clone:"shadowcopy"`
	// FeatureFlags is a map of feature flags to enable.
	FeatureFlags *xsync.MapOf[string, string] `clone:"shadowcopy"`
	// Options to use engine for running IaC operations.
	Engine *EngineOptions
	// Telemetry are telemetry options.
	Telemetry *telemetry.Options
	// Attributes to override in AWS provider nested within modules as part of the aws-provider-patch command.
	AwsProviderPatchOverrides map[string]string
	// A command that can be used to run Terragrunt with the given options.
	RunTerragrunt func(ctx context.Context, l log.Logger, opts *TerragruntOptions, r *report.Report) error
	// Version of terraform (obtained by running 'terraform version')
	TerraformVersion *version.Version `clone:"shadowcopy"`
	// Errors is a configuration for error handling.
	Errors *ErrorsConfig
	// Map to replace terraform source locations.
	SourceMap map[string]string
	// Environment variables at runtime
	Env map[string]string
	// StackAction is the action that should be performed on the stack.
	StackAction string
	// IAM Role options that should be used when authenticating to AWS.
	IAMRoleOptions IAMRoleOptions
	// IAM Role options set from command line.
	OriginalIAMRoleOptions IAMRoleOptions
	// The Token for authentication to the Terragrunt Provider Cache server.
	ProviderCacheToken string
	// Current Terraform command being executed by Terragrunt
	TerraformCommand string
	// StackOutputFormat format how the stack output is rendered.
	StackOutputFormat         string
	TerragruntStackConfigPath string
	// Location of the original Terragrunt config file.
	OriginalTerragruntConfigPath string
	// Unlike `WorkingDir`, this path is the same for all dependencies and points to the root working directory specified in the CLI.
	RootWorkingDir string
	// Download Terraform configurations from the specified source location into a temporary folder
	Source string
	// The working directory in which to run Terraform
	WorkingDir string
	// Location (or name) of the OpenTofu/Terraform binary
	TFPath string
	// Download Terraform configurations specified in the Source parameter into this folder
	DownloadDir string
	// Original Terraform command being executed by Terragrunt.
	OriginalTerraformCommand string
	// Terraform implementation tool (e.g. terraform, tofu) that terragrunt is wrapping
	TerraformImplementation TerraformImplementationType
	// The file path that terragrunt should use when rendering the terragrunt.hcl config as json.
	JSONOut string
	// The path to store unpacked providers.
	ProviderCacheDir string
	// Custom log level for engine
	EngineLogLevel string
	// Path to cache directory for engine files
	EngineCachePath string
	// The command and arguments that can be used to fetch authentication configurations.
	AuthProviderCmd string
	// Folder to store JSON representation of output files.
	JSONOutputFolder string
	// Folder to store output files.
	OutputFolder string
	// The file which hclfmt should be specifically run on
	HclFile string
	// The hostname of the Terragrunt Provider Cache server.
	ProviderCacheHostname string
	// Location of the Terragrunt config file
	TerragruntConfigPath string
	// Name of the root Terragrunt configuration file, if used.
	ScaffoldRootFileName string
	// Path to a file with a list of directories that need to be excluded when running *-all commands.
	ExcludesFile string
	// Path to folder of scaffold output
	ScaffoldOutputFolder string
	// Root directory for graph command.
	GraphRoot string
	// Path to the report file.
	ReportFile string
	// Report format.
	ReportFormat report.Format
	// Path to the report schema file.
	ReportSchemaFile string
	// CLI args that are intended for Terraform (i.e. all the CLI args except the --terragrunt ones)
	TerraformCliArgs cli.Args
	// Unix-style glob of directories to include when running *-all commands
	IncludeDirs []string
	// Unix-style glob of directories to exclude when running *-all commands
	ExcludeDirs []string
	// Files with variables to be used in modules scaffolding.
	ScaffoldVarFiles []string
	// The list of remote registries to cached by Terragrunt Provider Cache server.
	ProviderCacheRegistryNames []string
	// If set hclfmt will skip files in given directories.
	HclExclude []string
	// Variables for usage in scaffolding.
	ScaffoldVars []string
	// StrictControls is a slice of strict controls.
	StrictControls strict.Controls `clone:"shadowcopy"`
	// When used with `run --all`, restrict the modules in the stack to only those that include at least one of the files in this list.
	ModulesThatInclude []string
	// When used with `run --all`, restrict the units in the stack to only those that read at least one of the files in this list.
	UnitsReading []string
	// FilterQueries contains filter query strings for component selection
	FilterQueries []string
	// When set, it will be used to compute the cache key for `-version` checks.
	VersionManagerFileName []string
	// Experiments is a map of experiments, and their status.
	Experiments experiment.Experiments `clone:"shadowcopy"`
	// Parallelism limits the number of commands to run concurrently during *-all commands
	Parallelism int
	// When searching the directory tree, this is the max folders to check before exiting with an error.
	MaxFoldersToCheck int
	// The port of the Terragrunt Provider Cache server.
	ProviderCachePort int
	// Output Terragrunt logs in JSON format
	JSONLogFormat bool
	// True if terragrunt should run in debug mode
	Debug bool
	// Disable TF output formatting
	ForwardTFStdout bool
	// Fail execution if is required to create S3 bucket
	FailIfBucketCreationRequired bool
	// FilterAllowDestroy allows destroy runs when using Git-based filters
	FilterAllowDestroy bool
	// Controls if s3 bucket should be updated or skipped
	DisableBucketUpdate bool
	// Disables validation terraform command
	DisableCommandValidation bool
	// If True then HCL from StdIn must should be formatted.
	HclFromStdin bool
	// Show diff, by default it's disabled.
	Diff bool
	// Do not include root unit in scaffolding.
	ScaffoldNoIncludeRoot bool
	// Enable check mode, by default it's disabled.
	Check bool
	// Enables caching of includes during partial parsing operations.
	UsePartialParseConfigCache bool
	// If set to true, do not include dependencies when processing IncludeDirs
	StrictInclude bool
	// Disable listing of dependent modules in render json output
	JSONDisableDependentModules bool
	// Enables Terragrunt's provider caching.
	ProviderCache bool
	// If set to true, exclude all directories by default when running *-all commands
	ExcludeByDefault bool
	// This is an experimental feature, used to speed up dependency processing by getting the output from the state
	FetchDependencyOutputFromState bool
	// True if is required to show dependent modules and confirm action
	CheckDependentModules bool
	// True if is required not to show dependent modules and confirm action
	NoDestroyDependenciesCheck bool
	// Include fields metadata in render-json
	RenderJSONWithMetadata bool
	// Whether we should automatically retry errored Terraform commands
	AutoRetry bool
	// Flag to enable engine for running IaC operations.
	EngineEnabled bool
	// Whether we should automatically run terraform init if necessary when executing other commands
	AutoInit bool
	// Allows to skip the output of all dependencies.
	SkipOutput bool
	// Whether we should prompt the user for confirmation or always assume "yes"
	NonInteractive bool
	// If set to true, apply all external dependencies when running *-all commands
	IncludeExternalDependencies bool
	// Skip checksum check for engine package.
	EngineSkipChecksumCheck bool
	// If set to true, skip any external dependencies when running *-all commands
	IgnoreExternalDependencies bool
	// If set to true, ignore the dependency order when running *-all command.
	IgnoreDependencyOrder bool
	// If set to true, continue running *-all commands even if a dependency has errors.
	IgnoreDependencyErrors bool
	// Whether we should automatically run terraform with -auto-apply in run --all mode.
	RunAllAutoApprove bool
	// If set to true, delete the contents of the temporary folder before downloading Terraform source code into it
	SourceUpdate bool
	// HCLValidateStrict is a strict mode for HCL validation files. When it's set to false the command will only return an error if required inputs are missing from all input sources (env vars, var files, etc). When it's set to true, an error will be returned if required inputs are missing or if unused variables are passed to Terragrunt.",
	HCLValidateStrict bool
	// HCLValidateInputs checks if the terragrunt configured inputs align with the terraform defined variables.
	HCLValidateInputs bool
	// HCLValidateShowConfigPath shows the paths of the hcl invalid configs.
	HCLValidateShowConfigPath bool
	// HCLValidateJSONOutput outputs the hcl validate result as a JSON string.
	HCLValidateJSONOutput bool
	// If true, logs will be displayed in formatter key/value, by default logs are formatted in human-readable formatter.
	DisableLogFormatting bool
	// Headless is set when Terragrunt is running in headless mode.
	Headless bool
	// LogDisableErrorSummary is a flag to skip the error summary
	LogDisableErrorSummary bool
	// Disable replacing full paths in logs with short relative paths
	LogShowAbsPaths bool
	// NoStackGenerate disable stack generation.
	NoStackGenerate bool
	// NoStackValidate disable generated stack validation.
	NoStackValidate bool
	// RunAll runs the provided OpenTofu/Terraform command against a stack.
	RunAll bool
	// Graph runs the provided OpenTofu/Terraform against the graph of dependencies for the unit in the current working directory.
	Graph bool
	// BackendBootstrap automatically bootstraps backend infrastructure before attempting to use it.
	BackendBootstrap bool
	// DeleteBucket determines whether to delete entire bucket.
	DeleteBucket bool
	// ForceBackendDelete forces the backend to be deleted, even if the bucket is not versioned.
	ForceBackendDelete bool
	// ForceBackendMigrate forces the backend to be migrated, even if the bucket is not versioned.
	ForceBackendMigrate bool
	// SummaryDisable disables the summary output at the end of a run.
	SummaryDisable bool
	// SummaryPerUnit enables showing duration information for each unit in the summary.
	SummaryPerUnit bool
	// NoAutoProviderCacheDir disables the auto-provider-cache-dir feature even when the experiment is enabled.
	NoAutoProviderCacheDir bool
	// TFPathExplicitlySet is set to true if the user has explicitly set the TFPath via the --tf-path flag.
	TFPathExplicitlySet bool
	// FailFast is a flag to stop execution on the first error in apply of units.
	FailFast bool
	// NoDependencyPrompt disables prompt requiring confirmation for base and leaf file dependencies when using scaffolding.
	NoDependencyPrompt bool
	// NoShell disables shell commands when using boilerplate templates in catalog and scaffold commands.
	NoShell bool
	// NoHooks disables hooks when using boilerplate templates in catalog and scaffold commands.
	NoHooks bool
}

// TerragruntOptionsFunc is a functional option type used to pass options in certain integration tests
type TerragruntOptionsFunc func(*TerragruntOptions)

// WithIAMRoleARN adds the provided role ARN to IamRoleOptions
func WithIAMRoleARN(arn string) TerragruntOptionsFunc {
	return func(t *TerragruntOptions) {
		t.IAMRoleOptions.RoleARN = arn
	}
}

// WithIAMWebIdentityToken adds the provided WebIdentity token to IamRoleOptions
func WithIAMWebIdentityToken(token string) TerragruntOptionsFunc {
	return func(t *TerragruntOptions) {
		t.IAMRoleOptions.WebIdentityToken = token
	}
}

// IAMRoleOptions represents options that are used by Terragrunt to assume an IAM role.
type IAMRoleOptions struct {
	RoleARN               string
	WebIdentityToken      string
	AssumeRoleSessionName string
	AssumeRoleDuration    int64
}

func MergeIAMRoleOptions(target IAMRoleOptions, source IAMRoleOptions) IAMRoleOptions {
	out := target

	if source.RoleARN != "" {
		out.RoleARN = source.RoleARN
	}

	if source.AssumeRoleDuration != 0 {
		out.AssumeRoleDuration = source.AssumeRoleDuration
	}

	if source.AssumeRoleSessionName != "" {
		out.AssumeRoleSessionName = source.AssumeRoleSessionName
	}

	if source.WebIdentityToken != "" {
		out.WebIdentityToken = source.WebIdentityToken
	}

	return out
}

// NewTerragruntOptions creates a new TerragruntOptions object with
// reasonable defaults for real usage
func NewTerragruntOptions() *TerragruntOptions {
	return NewTerragruntOptionsWithWriters(os.Stdout, os.Stderr)
}

func NewTerragruntOptionsWithWriters(stdout, stderr io.Writer) *TerragruntOptions {
	return &TerragruntOptions{
		TFPath:                         DefaultWrappedPath,
		ExcludesFile:                   defaultExcludesFile,
		OriginalTerraformCommand:       "",
		TerraformCommand:               "",
		AutoInit:                       true,
		RunAllAutoApprove:              true,
		NonInteractive:                 false,
		TerraformCliArgs:               []string{},
		Env:                            map[string]string{},
		Source:                         "",
		SourceMap:                      map[string]string{},
		SourceUpdate:                   false,
		IgnoreDependencyErrors:         false,
		IgnoreDependencyOrder:          false,
		IgnoreExternalDependencies:     false,
		IncludeExternalDependencies:    false,
		Writer:                         stdout,
		ErrWriter:                      stderr,
		MaxFoldersToCheck:              DefaultMaxFoldersToCheck,
		AutoRetry:                      true,
		ExcludeDirs:                    []string{},
		IncludeDirs:                    []string{},
		ModulesThatInclude:             []string{},
		StrictInclude:                  false,
		Parallelism:                    DefaultParallelism,
		Check:                          false,
		Diff:                           false,
		FetchDependencyOutputFromState: false,
		UsePartialParseConfigCache:     false,
		ForwardTFStdout:                false,
		JSONOut:                        DefaultJSONOutName,
		TerraformImplementation:        UnknownImpl,
		JSONDisableDependentModules:    false,
		RunTerragrunt: func(ctx context.Context, l log.Logger, opts *TerragruntOptions, r *report.Report) error {
			return errors.New(ErrRunTerragruntCommandNotSet)
		},
		ProviderCacheRegistryNames: defaultProviderCacheRegistryNames,
		OutputFolder:               "",
		JSONOutputFolder:           "",
		FeatureFlags:               xsync.NewMapOf[string, string](),
		Errors:                     defaultErrorsConfig(),
		StrictControls:             controls.New(),
		Experiments:                experiment.NewExperiments(),
		Telemetry:                  new(telemetry.Options),
		NoStackValidate:            false,
		NoStackGenerate:            false,
		VersionManagerFileName:     defaultVersionManagerFileName,
		NoAutoProviderCacheDir:     false,
		NoDependencyPrompt:         false,
		NoShell:                    false,
		NoHooks:                    false,
	}
}

func NewTerragruntOptionsWithConfigPath(terragruntConfigPath string) (*TerragruntOptions, error) {
	opts := NewTerragruntOptions()
	opts.TerragruntConfigPath = terragruntConfigPath

	workingDir, downloadDir, err := DefaultWorkingAndDownloadDirs(terragruntConfigPath)
	if err != nil {
		return nil, errors.New(err)
	}

	opts.WorkingDir = workingDir
	opts.RootWorkingDir = workingDir
	opts.DownloadDir = downloadDir

	return opts, nil
}

// DefaultWorkingAndDownloadDirs gets the default working and download
// directories for the given Terragrunt config path.
func DefaultWorkingAndDownloadDirs(terragruntConfigPath string) (string, string, error) {
	workingDir := filepath.Dir(terragruntConfigPath)

	downloadDir, err := filepath.Abs(filepath.Join(workingDir, util.TerragruntCacheDir))
	if err != nil {
		return "", "", errors.New(err)
	}

	return filepath.ToSlash(workingDir), filepath.ToSlash(downloadDir), nil
}

// GetDefaultIAMAssumeRoleSessionName gets the default IAM assume role session name.
func GetDefaultIAMAssumeRoleSessionName() string {
	return fmt.Sprintf("terragrunt-%d", time.Now().UTC().UnixNano())
}

// NewTerragruntOptionsForTest creates a new TerragruntOptions object with reasonable defaults for test usage.
func NewTerragruntOptionsForTest(terragruntConfigPath string, options ...TerragruntOptionsFunc) (*TerragruntOptions, error) {
	formatter := format.NewFormatter(format.NewKeyValueFormatPlaceholders())
	formatter.SetDisabledColors(true)

	opts, err := NewTerragruntOptionsWithConfigPath(terragruntConfigPath)
	if err != nil {
		log.WithOptions(log.WithLevel(log.DebugLevel)).Errorf("%v\n", errors.New(err), log.WithFormatter(formatter))

		return nil, err
	}

	opts.NonInteractive = true

	for _, opt := range options {
		opt(opts)
	}

	return opts, nil
}

// OptionsFromContext tries to retrieve options from context, otherwise, returns its own instance.
func (opts *TerragruntOptions) OptionsFromContext(ctx context.Context) *TerragruntOptions {
	if val := ctx.Value(ContextKey); val != nil {
		if opts, ok := val.(*TerragruntOptions); ok {
			return opts
		}
	}

	return opts
}

// Clone performs a deep copy of `opts` with shadow copies of: interfaces, and funcs.
// Fields with "clone" tags can override this behavior.
func (opts *TerragruntOptions) Clone() *TerragruntOptions {
	newOpts := cloner.Clone(opts)

	return newOpts
}

// CloneWithConfigPath creates a copy of this TerragruntOptions, but with different values for the given variables. This is useful for
// creating a TerragruntOptions that behaves the same way, but is used for a Terraform module in a different folder.
//
// It also adjusts the given logger, as each cloned option has to use a working directory specific logger to enrich
// log output correctly.
func (opts *TerragruntOptions) CloneWithConfigPath(l log.Logger, configPath string) (log.Logger, *TerragruntOptions, error) {
	newOpts := opts.Clone()

	// Ensure configPath is absolute and normalized for consistent path handling
	configPath = util.CleanPath(configPath)
	if !filepath.IsAbs(configPath) {
		absConfigPath, err := filepath.Abs(configPath)
		if err != nil {
			return l, nil, err
		}

		configPath = util.CleanPath(absConfigPath)
	}

	workingDir := filepath.Dir(configPath)

	// Only update logger field if the working directory actually changed
	// This preserves any custom display path (e.g., relative path) set on the logger
	if workingDir != opts.WorkingDir {
		l = l.WithField(placeholders.WorkDirKeyName, workingDir)
	}

	newOpts.TerragruntConfigPath = configPath
	newOpts.WorkingDir = workingDir

	return l, newOpts, nil
}

// Check if argument is planfile TODO check file formatter
func checkIfPlanFile(arg string) bool {
	return util.IsFile(arg) && filepath.Ext(arg) == ".tfplan"
}

// Extract planfile from arguments list
func extractPlanFile(argsToInsert []string) (*string, []string) {
	planFile := ""

	var filteredArgs []string

	for _, arg := range argsToInsert {
		if checkIfPlanFile(arg) {
			planFile = arg
		} else {
			filteredArgs = append(filteredArgs, arg)
		}
	}

	if planFile != "" {
		return &planFile, filteredArgs
	}

	return nil, filteredArgs
}

// InsertTerraformCliArgs inserts the given argsToInsert after the terraform command argument, but before the remaining args.
func (opts *TerragruntOptions) InsertTerraformCliArgs(argsToInsert ...string) {
	planFile, restArgs := extractPlanFile(argsToInsert)

	commandLength := 1
	if util.ListContainsElement(TerraformCommandsWithSubcommand, opts.TerraformCliArgs[0]) {
		// Since these terraform commands require subcommands which may not always be properly passed by the user,
		// using util.Min to return the minimum to avoid potential out of bounds slice errors.
		commandLength = util.Min(minCommandLength, len(opts.TerraformCliArgs))
	}

	// Options must be inserted after command but before the other args
	// command is either 1 word or 2 words
	var args []string

	args = append(args, opts.TerraformCliArgs[:commandLength]...)
	args = append(args, restArgs...)
	args = append(args, opts.TerraformCliArgs[commandLength:]...)

	// check if planfile was extracted
	if planFile != nil {
		args = append(args, *planFile)
	}

	opts.TerraformCliArgs = args
}

// AppendTerraformCliArgs appends the given argsToAppend after the current TerraformCliArgs.
func (opts *TerragruntOptions) AppendTerraformCliArgs(argsToAppend ...string) {
	opts.TerraformCliArgs = append(opts.TerraformCliArgs, argsToAppend...)
}

// TerraformDataDir returns Terraform data directory (.terraform by default, overridden by $TF_DATA_DIR envvar)
func (opts *TerragruntOptions) TerraformDataDir() string {
	if tfDataDir, ok := opts.Env["TF_DATA_DIR"]; ok {
		return tfDataDir
	}

	return DefaultTFDataDir
}

// DataDir returns the Terraform data directory prepended with the working directory path,
// or just the Terraform data directory if it is an absolute path.
func (opts *TerragruntOptions) DataDir() string {
	tfDataDir := opts.TerraformDataDir()
	if filepath.IsAbs(tfDataDir) {
		return tfDataDir
	}

	return util.JoinPath(opts.WorkingDir, tfDataDir)
}

// identifyDefaultWrappedExecutable returns default path used for wrapped executable.
func identifyDefaultWrappedExecutable(ctx context.Context) string {
	if util.IsCommandExecutable(ctx, TofuDefaultPath, "-version") {
		return TofuDefaultPath
	}
	// fallback to Terraform if tofu is not available
	return TerraformDefaultPath
}

// EngineOptions Options for the Terragrunt engine.
type EngineOptions struct {
	Meta    map[string]any
	Source  string
	Version string
	Type    string
}

// ErrorsConfig extracted errors handling configuration.
type ErrorsConfig struct {
	Retry  map[string]*RetryConfig
	Ignore map[string]*IgnoreConfig
}

// RetryConfig represents the configuration for retrying specific errors.
type RetryConfig struct {
	Name             string
	RetryableErrors  []*ErrorsPattern
	MaxAttempts      int
	SleepIntervalSec int
}

// IgnoreConfig represents the configuration for ignoring specific errors.
type IgnoreConfig struct {
	Signals         map[string]any
	Name            string
	Message         string
	IgnorableErrors []*ErrorsPattern
}

type ErrorsPattern struct {
	Pattern  *regexp.Regexp `clone:"shadowcopy"`
	Negative bool
}

// RunWithErrorHandling runs the given operation and handles any errors according to the configuration.
func (opts *TerragruntOptions) RunWithErrorHandling(ctx context.Context, l log.Logger, r *report.Report, operation func() error) error {
	if opts.Errors == nil {
		return operation()
	}

	currentAttempt := 1

	// convert working dir to an absolute path for reporting
	absWorkingDir, err := filepath.Abs(opts.WorkingDir)
	if err != nil {
		return err
	}

	for {
		err := operation()
		if err == nil {
			return nil
		}

		// Process the error through our error handling configuration
		action, processErr := opts.Errors.ProcessError(l, err, currentAttempt)
		if processErr != nil {
			return fmt.Errorf("error processing error handling rules: %w", processErr)
		}

		if action == nil {
			return err
		}

		if action.ShouldIgnore {
			l.Warnf("Ignoring error, reason: %s", action.IgnoreMessage)

			// Handle ignore signals if any are configured
			if len(action.IgnoreSignals) > 0 {
				if err := opts.handleIgnoreSignals(l, action.IgnoreSignals); err != nil {
					return err
				}
			}

			run, err := r.EnsureRun(absWorkingDir)
			if err != nil {
				return err
			}

			if err := r.EndRun(
				run.Path,
				report.WithResult(report.ResultSucceeded),
				report.WithReason(report.ReasonErrorIgnored),
				report.WithCauseIgnoreBlock(action.IgnoreBlockName),
			); err != nil {
				return err
			}

			return nil
		}

		if action.ShouldRetry {
			// Respect --no-auto-retry flag
			if !opts.AutoRetry {
				return err
			}

			l.Warnf(
				"Encountered retryable error: %s\nAttempt %d of %d. Waiting %d second(s) before retrying...",
				action.RetryMessage,
				currentAttempt,
				action.RetryAttempts,
				action.RetrySleepSecs,
			)

			// Record that a retry will be attempted without prematurely marking success.
			run, err := r.EnsureRun(absWorkingDir)
			if err != nil {
				return err
			}

			if err := r.EndRun(
				run.Path,
				report.WithResult(report.ResultSucceeded),
				report.WithReason(report.ReasonRetrySucceeded),
				report.WithCauseRetryBlock(action.RetryBlockName),
			); err != nil {
				return err
			}

			// Sleep before retry
			select {
			case <-time.After(time.Duration(action.RetrySleepSecs) * time.Second):
				// try again
			case <-ctx.Done():
				return errors.New(ctx.Err())
			}

			currentAttempt++

			continue
		}

		return err
	}
}

func (opts *TerragruntOptions) handleIgnoreSignals(l log.Logger, signals map[string]any) error {
	workingDir := opts.WorkingDir
	signalsFile := filepath.Join(workingDir, DefaultSignalsFile)

	signalsJSON, err := json.MarshalIndent(signals, "", "  ")
	if err != nil {
		return err
	}

	const ownerPerms = 0644

	l.Warnf("Writing error signals to %s", signalsFile)

	if err := os.WriteFile(signalsFile, signalsJSON, ownerPerms); err != nil {
		return fmt.Errorf("failed to write signals file %s: %w", signalsFile, err)
	}

	return nil
}

// ErrorAction represents the action to take when an error occurs
type ErrorAction struct {
	IgnoreSignals   map[string]any
	IgnoreBlockName string
	RetryBlockName  string
	IgnoreMessage   string
	RetryMessage    string
	RetryAttempts   int
	RetrySleepSecs  int
	ShouldIgnore    bool
	ShouldRetry     bool
}

// ProcessError evaluates an error against the configuration and returns the appropriate action
func (c *ErrorsConfig) ProcessError(l log.Logger, err error, currentAttempt int) (*ErrorAction, error) {
	if err == nil {
		return nil, nil
	}

	errStr := extractErrorMessage(err)
	action := &ErrorAction{}

	l.Debugf("Processing error message: %s", errStr)

	// First check ignore rules
	for _, ignoreBlock := range c.Ignore {
		isIgnorable := matchesAnyRegexpPattern(errStr, ignoreBlock.IgnorableErrors)
		if isIgnorable {
			action.IgnoreBlockName = ignoreBlock.Name
			action.ShouldIgnore = true
			action.IgnoreMessage = ignoreBlock.Message
			action.IgnoreSignals = make(map[string]any)

			// Convert cty.Value map to regular map
			maps.Copy(action.IgnoreSignals, ignoreBlock.Signals)

			return action, nil
		}
	}

	// Then check retry rules
	for _, retryBlock := range c.Retry {
		isRetryable := matchesAnyRegexpPattern(errStr, retryBlock.RetryableErrors)
		if isRetryable {
			if currentAttempt >= retryBlock.MaxAttempts {
				return nil, errors.New(fmt.Sprintf("max retry attempts (%d) reached for error: %v",
					retryBlock.MaxAttempts, err))
			}

			action.RetryMessage = retryBlock.Name
			action.ShouldRetry = true
			action.RetryAttempts = retryBlock.MaxAttempts
			action.RetrySleepSecs = retryBlock.SleepIntervalSec

			return action, nil
		}
	}

	return nil, err
}

func extractErrorMessage(err error) string {
	// fetch the error string and remove any ASCII escape sequences
	multilineText := log.RemoveAllASCISeq(err.Error())
	errorText := errorCleanPattern.ReplaceAllString(multilineText, " ")

	return strings.Join(strings.Fields(errorText), " ")
}

// matchesAnyRegexpPattern checks if the input string matches any of the provided compiled patterns
func matchesAnyRegexpPattern(input string, patterns []*ErrorsPattern) bool {
	for _, pattern := range patterns {
		isNegative := pattern.Negative
		matched := pattern.Pattern.MatchString(input)

		if matched {
			return !isNegative
		}
	}

	return false
}

// ErrRunTerragruntCommandNotSet is a custom error type indicating that the command is not set.
var ErrRunTerragruntCommandNotSet = errors.New("the RunTerragrunt option has not been set on this TerragruntOptions object")
