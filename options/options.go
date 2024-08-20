// Package options provides a set of options that configure the behavior of the Terragrunt program.
package options

import (
	"context"
	goErrors "errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"time"

	"github.com/gruntwork-io/go-commons/errors"
	"github.com/gruntwork-io/terragrunt/internal/log/formatter"
	"github.com/gruntwork-io/terragrunt/util"
	"github.com/hashicorp/go-version"
	"github.com/sirupsen/logrus"
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

	DefaultTFDataDir = ".terraform"

	DefaultIAMAssumeRoleDuration = 3600

	minCommandLength = 2

	defaultExcludesFile = ".terragrunt-excludes"
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

	// Relative path to `RootWorkingDir`. We use this path for logs to shorten the path length.
	RelativeTerragruntConfigPath string

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
	Logger *logrus.Entry

	// Disable Terragrunt colors
	DisableLogColors bool

	// Output Terragrunt logs in JSON format
	JSONLogFormat bool

	// Wrap Terraform logs in JSON format
	TerraformLogsToJSON bool

	// Log level
	LogLevel logrus.Level

	// Raw log level value
	LogLevelStr string

	// If true, logs will be displayed in format key/value, by default logs are formatted in human-readable format.
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

	// The file path that terragrunt should use when rendering the terragrunt.hcl config as json.
	JSONOut string

	// When used with `run-all`, restrict the modules in the stack to only those that include at least one of the files
	// in this list.
	ModulesThatInclude []string

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

	// This is an experimental feature, used to speed up dependency processing by getting the output from the state
	FetchDependencyOutputFromState bool

	// Enables caching of includes during partial parsing operations.
	UsePartialParseConfigCache bool

	// Include fields metadata in render-json
	RenderJSONithMetadata bool

	// Prefix for shell commands' outputs
	OutputPrefix string

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

	// Options to use engine for running IaC operations.
	Engine *EngineOptions

	// LogPrefixStyle stores unique prefixes with their color schemes. When we clone the TerragruntOptions instance and create a new Logger we need to pass this cache to assign the same color to the prefix if it has been already discovered before.
	// Since TerragruntOptions can be cloned multiple times and branched as a tree, we always need to have access to the same value from all instances, so we use a pointer.
	LogPrefixStyle formatter.PrefixStyle
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
	return &TerragruntOptions{
		TerraformPath:                  DefaultWrappedPath,
		ExcludesFile:                   defaultExcludesFile,
		OriginalTerraformCommand:       "",
		TerraformCommand:               "",
		AutoInit:                       true,
		RunAllAutoApprove:              true,
		NonInteractive:                 false,
		TerraformCliArgs:               []string{},
		LogLevelStr:                    util.GetDefaultLogLevel().String(),
		Logger:                         util.GlobalFallbackLogEntry,
		Env:                            map[string]string{},
		Source:                         "",
		SourceMap:                      map[string]string{},
		SourceUpdate:                   false,
		IgnoreDependencyErrors:         false,
		IgnoreDependencyOrder:          false,
		IgnoreExternalDependencies:     false,
		IncludeExternalDependencies:    false,
		Writer:                         os.Stdout,
		ErrWriter:                      os.Stderr,
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
		OutputPrefix:                   "",
		ForwardTFStdout:                false,
		JSONOut:                        DefaultJSONOutName,
		TerraformImplementation:        UnknownImpl,
		JSONLogFormat:                  false,
		TerraformLogsToJSON:            false,
		JSONDisableDependentModules:    false,
		RunTerragrunt: func(ctx context.Context, opts *TerragruntOptions) error {
			return errors.WithStackTrace(ErrRunTerragruntCommandNotSet)
		},
		ProviderCacheRegistryNames: defaultProviderCacheRegistryNames,
		OutputFolder:               "",
		JSONOutputFolder:           "",
		LogPrefixStyle:             formatter.NewPrefixStyle(),
	}
}

func NewTerragruntOptionsWithConfigPath(terragruntConfigPath string) (*TerragruntOptions, error) {
	opts := NewTerragruntOptions()
	opts.TerragruntConfigPath = terragruntConfigPath

	workingDir, downloadDir, err := DefaultWorkingAndDownloadDirs(terragruntConfigPath)
	if err != nil {
		return nil, errors.WithStackTrace(err)
	}

	opts.WorkingDir = workingDir
	opts.DownloadDir = downloadDir

	return opts, nil
}

// DefaultWorkingAndDownloadDirs gets the default working and download
// directories for the given Terragrunt config path.
func DefaultWorkingAndDownloadDirs(terragruntConfigPath string) (string, string, error) {
	workingDir := filepath.Dir(terragruntConfigPath)

	downloadDir, err := filepath.Abs(filepath.Join(workingDir, util.TerragruntCacheDir))
	if err != nil {
		return "", "", errors.WithStackTrace(err)
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
		logger := util.CreateLogEntry("", util.GetDefaultLogLevel(), nil, opts.DisableLogColors, opts.DisableLogFormatting)
		logger.Errorf("%v\n", errors.WithStackTrace(err))

		return nil, err
	}

	opts.NonInteractive = true
	opts.Logger = util.CreateLogEntry("", logrus.DebugLevel, nil, opts.DisableLogColors, opts.DisableLogFormatting)
	opts.LogLevel = logrus.DebugLevel

	for _, opt := range options {
		opt(opts)
	}

	return opts, nil
}

// OptionsFromContext tries to retrieve options from context, otherwise, returns its own instance.
func (t *TerragruntOptions) OptionsFromContext(ctx context.Context) *TerragruntOptions {
	if val := ctx.Value(ContextKey); val != nil {
		if opts, ok := val.(*TerragruntOptions); ok {
			return opts
		}
	}

	return t
}

// Clone creates a copy of this TerragruntOptions, but with different values for the given variables. This is useful for
// creating a TerragruntOptions that behaves the same way, but is used for a Terraform module in a different folder.
func (t *TerragruntOptions) Clone(terragruntConfigPath string) (*TerragruntOptions, error) {
	workingDir := filepath.Dir(terragruntConfigPath)

	relTerragruntConfigPath, err := util.GetPathRelativeToWithSeparator(terragruntConfigPath, t.RootWorkingDir)
	if err != nil {
		return nil, err
	}

	var outputPrefix string
	if filepath.Dir(terragruntConfigPath) != t.RootWorkingDir {
		outputPrefix = filepath.Dir(relTerragruntConfigPath)
	}

	logger := util.CreateLogEntryWithWriter(t.ErrWriter, outputPrefix, t.LogLevel, t.Logger.Logger.Hooks, t.LogPrefixStyle, t.DisableLogColors, t.DisableLogFormatting)

	// Note that we clone lists and maps below as TerragruntOptions may be used and modified concurrently in the code
	// during xxx-all commands (e.g., apply-all, plan-all). See https://github.com/gruntwork-io/terragrunt/issues/367
	// for more info.
	return &TerragruntOptions{
		TerragruntConfigPath:           terragruntConfigPath,
		RelativeTerragruntConfigPath:   relTerragruntConfigPath,
		OriginalTerragruntConfigPath:   t.OriginalTerragruntConfigPath,
		TerraformPath:                  t.TerraformPath,
		OriginalTerraformCommand:       t.OriginalTerraformCommand,
		TerraformCommand:               t.TerraformCommand,
		TerraformVersion:               t.TerraformVersion,
		TerragruntVersion:              t.TerragruntVersion,
		AutoInit:                       t.AutoInit,
		RunAllAutoApprove:              t.RunAllAutoApprove,
		NonInteractive:                 t.NonInteractive,
		TerraformCliArgs:               util.CloneStringList(t.TerraformCliArgs),
		WorkingDir:                     workingDir,
		RootWorkingDir:                 t.RootWorkingDir,
		Logger:                         logger,
		LogLevel:                       t.LogLevel,
		LogPrefixStyle:                 t.LogPrefixStyle,
		ValidateStrict:                 t.ValidateStrict,
		Env:                            util.CloneStringMap(t.Env),
		Source:                         t.Source,
		SourceMap:                      t.SourceMap,
		SourceUpdate:                   t.SourceUpdate,
		DownloadDir:                    t.DownloadDir,
		Debug:                          t.Debug,
		OriginalIAMRoleOptions:         t.OriginalIAMRoleOptions,
		IAMRoleOptions:                 t.IAMRoleOptions,
		IgnoreDependencyErrors:         t.IgnoreDependencyErrors,
		IgnoreDependencyOrder:          t.IgnoreDependencyOrder,
		IgnoreExternalDependencies:     t.IgnoreExternalDependencies,
		IncludeExternalDependencies:    t.IncludeExternalDependencies,
		Writer:                         t.Writer,
		ErrWriter:                      t.ErrWriter,
		MaxFoldersToCheck:              t.MaxFoldersToCheck,
		AutoRetry:                      t.AutoRetry,
		RetryMaxAttempts:               t.RetryMaxAttempts,
		RetrySleepInterval:             t.RetrySleepInterval,
		RetryableErrors:                util.CloneStringList(t.RetryableErrors),
		ExcludesFile:                   t.ExcludesFile,
		ExcludeDirs:                    t.ExcludeDirs,
		IncludeDirs:                    t.IncludeDirs,
		ExcludeByDefault:               t.ExcludeByDefault,
		ModulesThatInclude:             t.ModulesThatInclude,
		Parallelism:                    t.Parallelism,
		StrictInclude:                  t.StrictInclude,
		RunTerragrunt:                  t.RunTerragrunt,
		AwsProviderPatchOverrides:      t.AwsProviderPatchOverrides,
		HclFile:                        t.HclFile,
		JSONOut:                        t.JSONOut,
		Check:                          t.Check,
		CheckDependentModules:          t.CheckDependentModules,
		FetchDependencyOutputFromState: t.FetchDependencyOutputFromState,
		UsePartialParseConfigCache:     t.UsePartialParseConfigCache,
		OutputPrefix:                   outputPrefix,
		ForwardTFStdout:                t.ForwardTFStdout,
		DisableLogFormatting:           t.DisableLogFormatting,
		FailIfBucketCreationRequired:   t.FailIfBucketCreationRequired,
		DisableBucketUpdate:            t.DisableBucketUpdate,
		TerraformImplementation:        t.TerraformImplementation,
		JSONLogFormat:                  t.JSONLogFormat,
		TerraformLogsToJSON:            t.TerraformLogsToJSON,
		GraphRoot:                      t.GraphRoot,
		ScaffoldVars:                   t.ScaffoldVars,
		ScaffoldVarFiles:               t.ScaffoldVarFiles,
		JSONDisableDependentModules:    t.JSONDisableDependentModules,
		ProviderCache:                  t.ProviderCache,
		ProviderCacheToken:             t.ProviderCacheToken,
		ProviderCacheDir:               t.ProviderCacheDir,
		ProviderCacheRegistryNames:     t.ProviderCacheRegistryNames,
		DisableLogColors:               t.DisableLogColors,
		OutputFolder:                   t.OutputFolder,
		JSONOutputFolder:               t.JSONOutputFolder,
		AuthProviderCmd:                t.AuthProviderCmd,
		SkipOutput:                     t.SkipOutput,
		Engine:                         cloneEngineOptions(t.Engine),
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

// Check if argument is planfile TODO check file format
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
func (t *TerragruntOptions) InsertTerraformCliArgs(argsToInsert ...string) {
	planFile, restArgs := extractPlanFile(argsToInsert)

	commandLength := 1
	if util.ListContainsElement(TerraformCommandsWithSubcommand, t.TerraformCliArgs[0]) {
		// Since these terraform commands require subcommands which may not always be properly passed by the user,
		// using util.Min to return the minimum to avoid potential out of bounds slice errors.
		commandLength = util.Min(minCommandLength, len(t.TerraformCliArgs))
	}

	// Options must be inserted after command but before the other args
	// command is either 1 word or 2 words
	var args []string
	args = append(args, t.TerraformCliArgs[:commandLength]...)
	args = append(args, restArgs...)
	args = append(args, t.TerraformCliArgs[commandLength:]...)

	// check if planfile was extracted
	if planFile != nil {
		args = append(args, *planFile)
	}

	t.TerraformCliArgs = args
}

// AppendTerraformCliArgs appends the given argsToAppend after the current TerraformCliArgs.
func (t *TerragruntOptions) AppendTerraformCliArgs(argsToAppend ...string) {
	t.TerraformCliArgs = append(t.TerraformCliArgs, argsToAppend...)
}

// TerraformDataDir returns Terraform data directory (.terraform by default, overridden by $TF_DATA_DIR envvar)
func (t *TerragruntOptions) TerraformDataDir() string {
	if tfDataDir, ok := t.Env["TF_DATA_DIR"]; ok {
		return tfDataDir
	}

	return DefaultTFDataDir
}

// DataDir returns the Terraform data directory prepended with the working directory path,
// or just the Terraform data directory if it is an absolute path.
func (t *TerragruntOptions) DataDir() string {
	tfDataDir := t.TerraformDataDir()
	if filepath.IsAbs(tfDataDir) {
		return tfDataDir
	}

	return util.JoinPath(t.WorkingDir, tfDataDir)
}

// identifyDefaultWrappedExecutable - return default path used for wrapped executable
func identifyDefaultWrappedExecutable() string {
	if util.IsCommandExecutable(TofuDefaultPath, "-version") {
		return TofuDefaultPath
	}
	// fallback to Terraform if tofu is not available
	return TerraformDefaultPath
}

// EngineOptions Options for the Terragrunt engine
type EngineOptions struct {
	Source  string
	Version string
	Type    string
	Meta    map[string]interface{}
}

// Custom error types

var ErrRunTerragruntCommandNotSet = goErrors.New("the RunTerragrunt option has not been set on this TerragruntOptions object")
