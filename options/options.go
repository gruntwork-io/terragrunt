// Package options provides a set of options that configure the behavior of the Terragrunt program.
package options

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"time"

	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/internal/experiment"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/pkg/log/format"
	"github.com/gruntwork-io/terragrunt/pkg/log/format/placeholders"
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

	defaultLogLevel = log.InfoLevel
)

var (
	DefaultWrappedPath = identifyDefaultWrappedExecutable()

	defaultProviderCacheRegistryNames = []string{
		"registry.terraform.io",
		"registry.opentofu.org",
	}

	TerraformCommandsWithSubcommand = []string{
		"debug",
		"force-unlock",
		"state",
	}
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
	// Location of the Terragrunt config file
	TerragruntConfigPath string

	TerragruntStackConfigPath string

	// Location of the original Terragrunt config file. This is primarily useful when one Terragrunt config is being
	// read from another: e.g., if /terraform-code/terragrunt.hcl calls read_terragrunt_config("/foo/bar.hcl"),
	// and within bar.hcl, you call get_original_terragrunt_dir(), you'll get back /terraform-code.
	OriginalTerragruntConfigPath string

	// Version of terragrunt
	TerragruntVersion *version.Version

	// Location of the terraform binary
	TerraformPath string

	// Current Terraform command being executed by Terragrunt
	TerraformCommand string

	// Original Terraform command being executed by Terragrunt. Used to track command evolution as terragrunt chains
	// different commands together. For example, when retrieving dependencies, terragrunt will change the
	// TerraformCommand to `output` to run `terraform output`, which loses the context of the original command that was
	// run to fetch the dependency. This is a problem when mock_outputs is configured and we only allow mocks to be
	// returned on specific commands.
	// NOTE: For `xxx-all` commands, this will be set to the Terraform command, which would be `xxx`. For example,
	// if you run `apply-all` (which is a terragrunt command), this variable will be set to `apply`.
	OriginalTerraformCommand string

	// Terraform implementation tool (e.g. terraform, tofu) that terragrunt is wrapping
	TerraformImplementation TerraformImplementationType

	// Version of terraform (obtained by running 'terraform version')
	TerraformVersion *version.Version

	// Whether we should prompt the user for confirmation or always assume "yes"
	NonInteractive bool

	// Whether we should automatically run terraform init if necessary when executing other commands
	AutoInit bool

	// Whether we should automatically run terraform with -auto-apply in run-all mode.
	RunAllAutoApprove bool

	// CLI args that are intended for Terraform (i.e. all the CLI args except the --terragrunt ones)
	TerraformCliArgs []string

	// The working directory in which to run Terraform
	WorkingDir string

	// Unlike `WorkingDir`, this path is the same for all dependencies and points to the root working directory specified in the CLI.
	RootWorkingDir string

	// Basic log entry
	Logger log.Logger

	// Disable Terragrunt colors
	DisableLogColors bool

	// Output Terragrunt logs in JSON format
	JSONLogFormat bool

	// Disable replacing full paths in logs with short relative paths
	LogShowAbsPaths bool

	// Log level
	LogLevel log.Level

	// Log formatter
	LogFormatter *format.Formatter

	// If true, logs will be disabled
	DisableLog bool

	// If true, logs will be displayed in formatter key/value, by default logs are formatted in human-readable formatter.
	DisableLogFormatting bool

	// ValidateStrict mode for the validate-inputs command
	ValidateStrict bool

	// Environment variables at runtime
	Env map[string]string

	// Download Terraform configurations from the specified source location into a temporary folder and run
	// Terraform in that temporary folder
	Source string

	// Map to replace terraform source locations. This will replace occurrences of the given source with the target
	// value.
	SourceMap map[string]string

	// If set to true, delete the contents of the temporary folder before downloading Terraform source code into it
	SourceUpdate bool

	// Download Terraform configurations specified in the Source parameter into this folder
	DownloadDir string

	// IAM Role options set from command line. This is used to differentiate between the options set from the config and
	// CLI.
	OriginalIAMRoleOptions IAMRoleOptions

	// IAM Role options that should be used when authenticating to AWS.
	IAMRoleOptions IAMRoleOptions

	// If set to true, continue running *-all commands even if a dependency has errors. This is mostly useful for 'output-all <some_variable>'. See https://github.com/gruntwork-io/terragrunt/issues/193
	IgnoreDependencyErrors bool

	// If set to true, ignore the dependency order when running *-all command.
	IgnoreDependencyOrder bool

	// If set to true, skip any external dependencies when running *-all commands
	IgnoreExternalDependencies bool

	// If set to true, apply all external dependencies when running *-all commands
	IncludeExternalDependencies bool

	// If you want stdout to go somewhere other than os.stdout
	Writer io.Writer

	// If you want stderr to go somewhere other than os.stderr
	ErrWriter io.Writer

	// When searching the directory tree, this is the max folders to check before exiting with an error. This is
	// exposed here primarily so we can set it to a low value at test time.
	MaxFoldersToCheck int

	// Whether we should automatically retry errored Terraform commands
	AutoRetry bool

	// Maximum number of times to retry errors matching RetryableErrors
	RetryMaxAttempts int

	// The duration in seconds to wait before retrying
	RetrySleepInterval time.Duration

	// RetryableErrors is an array of regular expressions with RE2 syntax (https://github.com/google/re2/wiki/Syntax) that qualify for retrying
	RetryableErrors []string

	// Path to a file with a list of directories that need  to be excluded when running *-all commands.
	ExcludesFile string

	// Unix-style glob of directories to exclude when running *-all commands
	ExcludeDirs []string

	// Unix-style glob of directories to include when running *-all commands
	IncludeDirs []string

	// If set to true, exclude all directories by default when running *-all commands
	// Is set automatically if IncludeDirs is set
	ExcludeByDefault bool

	// If set to true, do not include dependencies when processing IncludeDirs (unless they are in the included dirs)
	StrictInclude bool

	// Parallelism limits the number of commands to run concurrently during *-all commands
	Parallelism int

	// Enable check mode, by default it's disabled.
	Check bool

	// Show diff, by default it's disabled.
	Diff bool

	// The file which hclfmt should be specifically run on
	HclFile string

	// If set hclfmt will skip files in given directories.
	HclExclude []string

	// If True then HCL from StdIn must should be formatted.
	HclFromStdin bool

	// The file path that terragrunt should use when rendering the terragrunt.hcl config as json.
	JSONOut string

	// When used with `run-all`, restrict the modules in the stack to only those that include at least one of the files
	// in this list.
	ModulesThatInclude []string

	// When used with `run-all`, restrict the units in the stack to only those that read at least one of the files
	// in this list.
	UnitsReading []string

	// A command that can be used to run Terragrunt with the given options. This is useful for running Terragrunt
	// multiple times (e.g. when spinning up a stack of Terraform modules). The actual command is normally defined
	// in the cli package, which depends on almost all other packages, so we declare it here so that other
	// packages can use the command without a direct reference back to the cli package (which would create a
	// circular dependency).
	RunTerragrunt func(ctx context.Context, opts *TerragruntOptions) error

	// True if terragrunt should run in debug mode, writing terragrunt-debug.tfvars to working folder to help
	// root-cause issues.
	Debug bool

	// Attributes to override in AWS provider nested within modules as part of the aws-provider-patch command. See that
	// command for more info.
	AwsProviderPatchOverrides map[string]string

	// True if is required to show dependent modules and confirm action
	CheckDependentModules bool

	// True if is required not to show dependent modules and confirm action
	NoDestroyDependenciesCheck bool

	// This is an experimental feature, used to speed up dependency processing by getting the output from the state
	FetchDependencyOutputFromState bool

	// Enables caching of includes during partial parsing operations.
	UsePartialParseConfigCache bool

	// Include fields metadata in render-json
	RenderJSONWithMetadata bool

	// Disable TF output formatting
	ForwardTFStdout bool

	// Fail execution if is required to create S3 bucket
	FailIfBucketCreationRequired bool

	// Controls if s3 bucket should be updated or skipped
	DisableBucketUpdate bool

	// Disables validation terraform command
	DisableCommandValidation bool

	// Variables for usage in scaffolding.
	ScaffoldVars []string

	// Files with variables to be used in modules scaffolding.
	ScaffoldVarFiles []string

	// Do not include root unit in scaffolding.
	ScaffoldNoIncludeRoot bool

	// Name of the root Terragrunt configuration file, if used.
	ScaffoldRootFileName string

	// Root directory for graph command.
	GraphRoot string

	// Disable listing of dependent modules in render json output
	JSONDisableDependentModules bool

	// Enables Terragrunt's provider caching.
	ProviderCache bool

	// The path to store unpacked providers. The file structure is the same as terraform plugin cache dir.
	ProviderCacheDir string

	// The Token for authentication to the Terragrunt Provider Cache server.
	ProviderCacheToken string

	// The hostname of the Terragrunt Provider Cache server.
	ProviderCacheHostname string

	// The port of the Terragrunt Provider Cache server.
	ProviderCachePort int

	// The list of remote registries to cached by Terragrunt Provider Cache server.
	ProviderCacheRegistryNames []string

	// Folder to store output files.
	OutputFolder string

	// Folder to store JSON representation of output files.
	JSONOutputFolder string

	// The command and arguments that can be used to fetch authentication configurations.
	// Terragrunt invokes this command before running tofu/terraform operations for each working directory.
	AuthProviderCmd string

	// Allows to skip the output of all dependencies. Intended for use with `hclvalidate` command.
	SkipOutput bool

	// Flag to enable engine for running IaC operations.
	EngineEnabled bool

	// Path to cache directory for engine files
	EngineCachePath string

	// Skip checksum check for engine package.
	EngineSkipChecksumCheck bool

	// Custom log level for engine
	EngineLogLevel string

	// Options to use engine for running IaC operations.
	Engine *EngineOptions

	// StrictMode is a flag to enable strict mode for terragrunt.
	StrictMode bool

	// StrictControls is a slice of strict controls enabled.
	StrictControls []string

	// ExperimentMode is a flag to enable experiment mode for terragrunt.
	ExperimentMode bool

	// Experiments is a map of experiments, and their status.
	Experiments experiment.Experiments

	// ]FeatureFlags is a map of feature flags to enable.
	FeatureFlags *xsync.MapOf[string, string]

	// ReadFiles is a map of files to the Units
	// that read them using HCL functions in the unit.
	ReadFiles *xsync.MapOf[string, []string]

	// Errors is a configuration for error handling.
	Errors *ErrorsConfig

	// Headless is set when Terragrunt is running in
	// headless mode. In this mode, Terragrunt will not
	// return stdout/stderr directly to the caller.
	//
	// It will instead write the output to INFO,
	// as it's not something intended for a user
	// to use in a programmatic way.
	Headless bool

	// LogDisableErrorSummary is a flag to skip the error summary
	// provided at the end of Terragrunt execution to
	// recap all that was emitted in stderr throughout
	// the run of an orchestrated process.
	LogDisableErrorSummary bool
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
	// The ARN of an IAM Role to assume. Used when accessing AWS, both internally and through terraform.
	RoleARN string

	// The Web identity token. Used when RoleArn is also set to use AssumeRoleWithWebIdentity instead of AssumeRole.
	WebIdentityToken string

	// Duration of the STS Session when assuming the role.
	AssumeRoleDuration int64

	// STS Session name when assuming the role.
	AssumeRoleSessionName string
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
	var logFormatter = format.NewFormatter(format.NewPrettyFormat())

	return &TerragruntOptions{
		TerraformPath:                  DefaultWrappedPath,
		ExcludesFile:                   defaultExcludesFile,
		OriginalTerraformCommand:       "",
		TerraformCommand:               "",
		AutoInit:                       true,
		RunAllAutoApprove:              true,
		NonInteractive:                 false,
		TerraformCliArgs:               []string{},
		LogLevel:                       defaultLogLevel,
		LogFormatter:                   logFormatter,
		Logger:                         log.New(log.WithOutput(stderr), log.WithLevel(defaultLogLevel), log.WithFormatter(logFormatter)),
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
		RetryMaxAttempts:               DefaultRetryMaxAttempts,
		RetrySleepInterval:             DefaultRetrySleepInterval,
		RetryableErrors:                util.CloneStringList(DefaultRetryableErrors),
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
		RunTerragrunt: func(ctx context.Context, opts *TerragruntOptions) error {
			return errors.New(ErrRunTerragruntCommandNotSet)
		},
		ProviderCacheRegistryNames: defaultProviderCacheRegistryNames,
		OutputFolder:               "",
		JSONOutputFolder:           "",
		FeatureFlags:               xsync.NewMapOf[string, string](),
		ReadFiles:                  xsync.NewMapOf[string, []string](),
		ExperimentMode:             false,
		Experiments:                experiment.NewExperiments(),
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
	opts, err := NewTerragruntOptionsWithConfigPath(terragruntConfigPath)
	if err != nil {
		log.WithOptions(log.WithLevel(log.DebugLevel)).Errorf("%v\n", errors.New(err))

		return nil, err
	}

	opts.NonInteractive = true
	opts.Logger.SetOptions(log.WithLevel(log.DebugLevel))
	opts.LogLevel = log.DebugLevel

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

// Clone creates a copy of this TerragruntOptions, but with different values for the given variables. This is useful for
// creating a TerragruntOptions that behaves the same way, but is used for a Terraform module in a different folder.
func (opts *TerragruntOptions) Clone(terragruntConfigPath string) (*TerragruntOptions, error) {
	workingDir := filepath.Dir(terragruntConfigPath)

	// Note that we clone lists and maps below as TerragruntOptions may be used and modified concurrently in the code
	// during xxx-all commands (e.g., apply-all, plan-all). See https://github.com/gruntwork-io/terragrunt/issues/367
	// for more info.
	return &TerragruntOptions{
		TerragruntConfigPath:         terragruntConfigPath,
		OriginalTerragruntConfigPath: opts.OriginalTerragruntConfigPath,
		TerraformPath:                opts.TerraformPath,
		OriginalTerraformCommand:     opts.OriginalTerraformCommand,
		TerraformCommand:             opts.TerraformCommand,
		TerraformVersion:             opts.TerraformVersion,
		TerragruntVersion:            opts.TerragruntVersion,
		AutoInit:                     opts.AutoInit,
		RunAllAutoApprove:            opts.RunAllAutoApprove,
		NonInteractive:               opts.NonInteractive,
		TerraformCliArgs:             util.CloneStringList(opts.TerraformCliArgs),
		WorkingDir:                   workingDir,
		RootWorkingDir:               opts.RootWorkingDir,
		Logger: opts.Logger.WithFields(log.Fields{
			placeholders.WorkDirKeyName:     workingDir,
			placeholders.DownloadDirKeyName: opts.DownloadDir,
		}),
		LogLevel:                       opts.LogLevel,
		LogFormatter:                   opts.LogFormatter,
		ValidateStrict:                 opts.ValidateStrict,
		Env:                            util.CloneStringMap(opts.Env),
		Source:                         opts.Source,
		SourceMap:                      opts.SourceMap,
		SourceUpdate:                   opts.SourceUpdate,
		DownloadDir:                    opts.DownloadDir,
		Debug:                          opts.Debug,
		OriginalIAMRoleOptions:         opts.OriginalIAMRoleOptions,
		IAMRoleOptions:                 opts.IAMRoleOptions,
		IgnoreDependencyErrors:         opts.IgnoreDependencyErrors,
		IgnoreDependencyOrder:          opts.IgnoreDependencyOrder,
		IgnoreExternalDependencies:     opts.IgnoreExternalDependencies,
		IncludeExternalDependencies:    opts.IncludeExternalDependencies,
		Writer:                         opts.Writer,
		ErrWriter:                      opts.ErrWriter,
		MaxFoldersToCheck:              opts.MaxFoldersToCheck,
		AutoRetry:                      opts.AutoRetry,
		RetryMaxAttempts:               opts.RetryMaxAttempts,
		RetrySleepInterval:             opts.RetrySleepInterval,
		RetryableErrors:                util.CloneStringList(opts.RetryableErrors),
		ExcludesFile:                   opts.ExcludesFile,
		ExcludeDirs:                    opts.ExcludeDirs,
		IncludeDirs:                    opts.IncludeDirs,
		ExcludeByDefault:               opts.ExcludeByDefault,
		ModulesThatInclude:             opts.ModulesThatInclude,
		UnitsReading:                   opts.UnitsReading,
		ReadFiles:                      opts.ReadFiles,
		Parallelism:                    opts.Parallelism,
		StrictInclude:                  opts.StrictInclude,
		RunTerragrunt:                  opts.RunTerragrunt,
		AwsProviderPatchOverrides:      opts.AwsProviderPatchOverrides,
		HclFile:                        opts.HclFile,
		HclExclude:                     opts.HclExclude,
		HclFromStdin:                   opts.HclFromStdin,
		JSONOut:                        opts.JSONOut,
		JSONLogFormat:                  opts.JSONLogFormat,
		Check:                          opts.Check,
		CheckDependentModules:          opts.CheckDependentModules,
		NoDestroyDependenciesCheck:     opts.NoDestroyDependenciesCheck,
		FetchDependencyOutputFromState: opts.FetchDependencyOutputFromState,
		UsePartialParseConfigCache:     opts.UsePartialParseConfigCache,
		ForwardTFStdout:                opts.ForwardTFStdout,
		FailIfBucketCreationRequired:   opts.FailIfBucketCreationRequired,
		DisableBucketUpdate:            opts.DisableBucketUpdate,
		TerraformImplementation:        opts.TerraformImplementation,
		GraphRoot:                      opts.GraphRoot,
		ScaffoldVars:                   opts.ScaffoldVars,
		ScaffoldVarFiles:               opts.ScaffoldVarFiles,
		JSONDisableDependentModules:    opts.JSONDisableDependentModules,
		ProviderCache:                  opts.ProviderCache,
		ProviderCacheToken:             opts.ProviderCacheToken,
		ProviderCacheDir:               opts.ProviderCacheDir,
		ProviderCacheRegistryNames:     opts.ProviderCacheRegistryNames,
		DisableLogColors:               opts.DisableLogColors,
		OutputFolder:                   opts.OutputFolder,
		JSONOutputFolder:               opts.JSONOutputFolder,
		AuthProviderCmd:                opts.AuthProviderCmd,
		SkipOutput:                     opts.SkipOutput,
		DisableLog:                     opts.DisableLog,
		EngineEnabled:                  opts.EngineEnabled,
		EngineCachePath:                opts.EngineCachePath,
		EngineLogLevel:                 opts.EngineLogLevel,
		EngineSkipChecksumCheck:        opts.EngineSkipChecksumCheck,
		Engine:                         cloneEngineOptions(opts.Engine),
		ExperimentMode:                 opts.ExperimentMode,
		// This doesn't have to be deep cloned, as the same experiments
		// are used across all units in a `run-all`. If that changes in
		// the future, we can deep clone this as well.
		Experiments: opts.Experiments,
		// copy array
		StrictControls:         util.CloneStringList(opts.StrictControls),
		FeatureFlags:           opts.FeatureFlags,
		Errors:                 cloneErrorsConfig(opts.Errors),
		ScaffoldNoIncludeRoot:  opts.ScaffoldNoIncludeRoot,
		ScaffoldRootFileName:   opts.ScaffoldRootFileName,
		Headless:               opts.Headless,
		LogDisableErrorSummary: opts.LogDisableErrorSummary,
	}, nil
}

// cloneEngineOptions creates a deep copy of the given EngineOptions
func cloneEngineOptions(opts *EngineOptions) *EngineOptions {
	if opts == nil {
		return nil
	}

	return &EngineOptions{
		Source:  opts.Source,
		Version: opts.Version,
		Type:    opts.Type,
		Meta:    opts.Meta,
	}
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

// AppendReadFile appends to the list of files read by a given unit.
func (opts *TerragruntOptions) AppendReadFile(file, unit string) {
	if opts.ReadFiles == nil {
		opts.ReadFiles = xsync.NewMapOf[string, []string]()
	}

	units, ok := opts.ReadFiles.Load(file)
	if !ok {
		opts.ReadFiles.Store(file, []string{unit})
		return
	}

	for _, u := range units {
		if u == unit {
			return
		}
	}

	opts.Logger.Debugf("Tracking that file %s was read by %s.", file, unit)

	// Atomic insert
	// https://github.com/puzpuzpuz/xsync/issues/123#issuecomment-1963458519
	_, _ = opts.ReadFiles.Compute(file, func(oldUnits []string, loaded bool) ([]string, bool) {
		var newUnits []string

		if loaded {
			newUnits = append(make([]string, 0, len(oldUnits)+1), oldUnits...)
			newUnits = append(newUnits, unit)
		} else {
			newUnits = []string{unit}
		}

		return newUnits, false
	})
}

// DidReadFile checks if a given file was read by a given unit.
func (opts *TerragruntOptions) DidReadFile(file, unit string) bool {
	if opts.ReadFiles == nil {
		return false
	}

	units, ok := opts.ReadFiles.Load(file)
	if !ok {
		return false
	}

	for _, u := range units {
		if u == unit {
			return true
		}
	}

	return false
}

// CloneReadFiles creates a copy of the ReadFiles map.
func (opts *TerragruntOptions) CloneReadFiles(readFiles *xsync.MapOf[string, []string]) {
	if readFiles == nil {
		return
	}

	readFiles.Range(func(key string, units []string) bool {
		for _, unit := range units {
			opts.AppendReadFile(key, unit)
		}

		return true
	})
}

// identifyDefaultWrappedExecutable returns default path used for wrapped executable.
func identifyDefaultWrappedExecutable() string {
	if util.IsCommandExecutable(TofuDefaultPath, "-version") {
		return TofuDefaultPath
	}
	// fallback to Terraform if tofu is not available
	return TerraformDefaultPath
}

// EngineOptions Options for the Terragrunt engine.
type EngineOptions struct {
	Source  string
	Version string
	Type    string
	Meta    map[string]interface{}
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
	Name            string
	IgnorableErrors []*ErrorsPattern
	Message         string
	Signals         map[string]interface{}
}

type ErrorsPattern struct {
	Pattern  *regexp.Regexp
	Negative bool
}

func cloneErrorsConfig(config *ErrorsConfig) *ErrorsConfig {
	if config == nil {
		return nil
	}

	// Create a new Errors
	cloned := &ErrorsConfig{
		Retry:  make(map[string]*RetryConfig),
		Ignore: make(map[string]*IgnoreConfig),
	}

	// Clone Retry configurations
	for key, retryConfig := range config.Retry {
		if retryConfig != nil {
			cloned.Retry[key] = &RetryConfig{
				Name:             retryConfig.Name,
				MaxAttempts:      retryConfig.MaxAttempts,
				SleepIntervalSec: retryConfig.SleepIntervalSec,
				RetryableErrors:  make([]*ErrorsPattern, len(retryConfig.RetryableErrors)),
			}
			// Deep copy the RetryableErrors slice
			copy(cloned.Retry[key].RetryableErrors, retryConfig.RetryableErrors)
		}
	}

	// Clone Ignore configurations
	for key, ignoreConfig := range config.Ignore {
		if ignoreConfig != nil {
			cloned.Ignore[key] = &IgnoreConfig{
				Name:            ignoreConfig.Name,
				Message:         ignoreConfig.Message,
				IgnorableErrors: make([]*ErrorsPattern, len(ignoreConfig.IgnorableErrors)),
				Signals:         make(map[string]interface{}),
			}
			// Deep copy the IgnorableErrors slice
			copy(cloned.Ignore[key].IgnorableErrors, ignoreConfig.IgnorableErrors)

			// Deep copy the Signals map
			for sigKey, sigVal := range ignoreConfig.Signals {
				cloned.Ignore[key].Signals[sigKey] = sigVal
			}
		}
	}

	return cloned
}

// RunWithErrorHandling runs the given operation and handles any errors according to the configuration.
func (opts *TerragruntOptions) RunWithErrorHandling(ctx context.Context, operation func() error) error {
	if opts.Errors == nil {
		return operation()
	}

	currentAttempt := 1

	for {
		err := operation()
		if err == nil {
			return nil
		}

		// Process the error through our error handling configuration
		action, processErr := opts.Errors.ProcessError(err, currentAttempt)
		if processErr != nil {
			return fmt.Errorf("error processing error handling rules: %w", processErr)
		}

		if action == nil {
			return err
		}

		if action.ShouldIgnore {
			opts.Logger.Warnf("Ignoring error, reason: %s", action.IgnoreMessage)

			// Handle ignore signals if any are configured
			if len(action.IgnoreSignals) > 0 {
				if err := opts.handleIgnoreSignals(action.IgnoreSignals); err != nil {
					return err
				}
			}

			return nil
		}

		if action.ShouldRetry {
			opts.Logger.Warnf(
				"Encountered retryable error: %s\nAttempt %d of %d. Waiting %d second(s) before retrying...",
				action.RetryMessage,
				currentAttempt,
				action.RetryAttempts,
				action.RetrySleepSecs,
			)

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

func (opts *TerragruntOptions) handleIgnoreSignals(signals map[string]interface{}) error {
	workingDir := opts.WorkingDir
	signalsFile := filepath.Join(workingDir, DefaultSignalsFile)
	signalsJSON, err := json.MarshalIndent(signals, "", "  ")

	if err != nil {
		return err
	}

	const ownerPerms = 0644
	if err := os.WriteFile(signalsFile, signalsJSON, ownerPerms); err != nil {
		return fmt.Errorf("failed to write signals file %s: %w", signalsFile, err)
	}

	opts.Logger.Warnf("Written error signals to %s", signalsFile)

	return nil
}

// ErrorAction represents the action to take when an error occurs
type ErrorAction struct {
	ShouldIgnore   bool
	ShouldRetry    bool
	IgnoreMessage  string
	IgnoreSignals  map[string]interface{}
	RetryMessage   string
	RetryAttempts  int
	RetrySleepSecs int
}

// ProcessError evaluates an error against the configuration and returns the appropriate action
func (c *ErrorsConfig) ProcessError(err error, currentAttempt int) (*ErrorAction, error) {
	if err == nil {
		return nil, nil
	}

	errStr := err.Error()
	action := &ErrorAction{}

	// First check ignore rules
	for _, ignoreBlock := range c.Ignore {
		isIgnorable := matchesAnyRegexpPattern(errStr, ignoreBlock.IgnorableErrors)
		if isIgnorable {
			action.ShouldIgnore = true
			action.IgnoreMessage = ignoreBlock.Message
			action.IgnoreSignals = make(map[string]interface{})

			// Convert cty.Value map to regular map
			for k, v := range ignoreBlock.Signals {
				action.IgnoreSignals[k] = v
			}

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
