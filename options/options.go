package options

import (
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"time"

	"github.com/gruntwork-io/terragrunt/errors"
	"github.com/gruntwork-io/terragrunt/util"
	"github.com/hashicorp/go-version"
	"github.com/sirupsen/logrus"
)

var TERRAFORM_COMMANDS_WITH_SUBCOMMAND = []string{
	"debug",
	"force-unlock",
	"state",
}

const DEFAULT_MAX_FOLDERS_TO_CHECK = 100

// no limits on parallelism by default (limited by GOPROCS)
const DEFAULT_PARALLELISM = math.MaxInt32

// TERRAFORM_DEFAULT_PATH just takes terraform from the path
const TERRAFORM_DEFAULT_PATH = "terraform"

const TerragruntCacheDir = ".terragrunt-cache"

const DefaultTFDataDir = ".terraform"

const DefaultIAMAssumeRoleDuration = 3600

// TerragruntOptions represents options that configure the behavior of the Terragrunt program
type TerragruntOptions struct {
	// Location of the Terragrunt config file
	TerragruntConfigPath string

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

	// Basic log entry
	Logger *logrus.Entry

	// Log level
	LogLevel logrus.Level

	// ValidateStrict mode for the validate-inputs command
	ValidateStrict bool

	// Environment variables at runtime
	Env map[string]string

	// Download Terraform configurations from the specified source location into a temporary folder and run
	// Terraform in that temporary folder
	Source string

	// Map to replace terraform source locations. This will replace occurences of the given source with the target
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
	RetrySleepIntervalSec time.Duration

	// RetryableErrors is an array of regular expressions with RE2 syntax (https://github.com/google/re2/wiki/Syntax) that qualify for retrying
	RetryableErrors []string

	// Unix-style glob of directories to exclude when running *-all commands
	ExcludeDirs []string

	// Unix-style glob of directories to include when running *-all commands
	IncludeDirs []string

	// If set to true, do not include dependencies when processing IncludeDirs (unless they are in the included dirs)
	StrictInclude bool

	// Parallelism limits the number of commands to run concurrently during *-all commands
	Parallelism int

	// Enable check mode, by default it's disabled.
	Check bool

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
	RunTerragrunt func(*TerragruntOptions) error

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
}

// IAMOptions represents options that are used by Terragrunt to assume an IAM role.
type IAMRoleOptions struct {
	// The ARN of an IAM Role to assume. Used when accessing AWS, both internally and through terraform.
	RoleARN string

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

	return out
}

// Create a new TerragruntOptions object with reasonable defaults for real usage
func NewTerragruntOptions(terragruntConfigPath string) (*TerragruntOptions, error) {
	defaultLogLevel := util.GetDefaultLogLevel()
	logger := util.CreateLogEntry("", defaultLogLevel)

	workingDir, downloadDir, err := DefaultWorkingAndDownloadDirs(terragruntConfigPath)
	if err != nil {
		return nil, errors.WithStackTrace(err)
	}

	return &TerragruntOptions{
		TerragruntConfigPath:           terragruntConfigPath,
		TerraformPath:                  TERRAFORM_DEFAULT_PATH,
		OriginalTerraformCommand:       "",
		TerraformCommand:               "",
		AutoInit:                       true,
		RunAllAutoApprove:              true,
		NonInteractive:                 false,
		TerraformCliArgs:               []string{},
		WorkingDir:                     workingDir,
		Logger:                         logger,
		LogLevel:                       defaultLogLevel,
		ValidateStrict:                 false,
		Env:                            map[string]string{},
		Source:                         "",
		SourceMap:                      map[string]string{},
		SourceUpdate:                   false,
		DownloadDir:                    downloadDir,
		IgnoreDependencyErrors:         false,
		IgnoreDependencyOrder:          false,
		IgnoreExternalDependencies:     false,
		IncludeExternalDependencies:    false,
		Writer:                         os.Stdout,
		ErrWriter:                      os.Stderr,
		MaxFoldersToCheck:              DEFAULT_MAX_FOLDERS_TO_CHECK,
		AutoRetry:                      true,
		RetryMaxAttempts:               DEFAULT_RETRY_MAX_ATTEMPTS,
		RetrySleepIntervalSec:          DEFAULT_RETRY_SLEEP_INTERVAL_SEC,
		RetryableErrors:                util.CloneStringList(DEFAULT_RETRYABLE_ERRORS),
		ExcludeDirs:                    []string{},
		IncludeDirs:                    []string{},
		ModulesThatInclude:             []string{},
		StrictInclude:                  false,
		Parallelism:                    DEFAULT_PARALLELISM,
		Check:                          false,
		FetchDependencyOutputFromState: false,
		RunTerragrunt: func(terragruntOptions *TerragruntOptions) error {
			return errors.WithStackTrace(RunTerragruntCommandNotSet)
		},
	}, nil
}

// Get the default working and download directories for the given Terragrunt config path
func DefaultWorkingAndDownloadDirs(terragruntConfigPath string) (string, string, error) {
	workingDir := filepath.Dir(terragruntConfigPath)

	downloadDir, err := filepath.Abs(filepath.Join(workingDir, TerragruntCacheDir))
	if err != nil {
		return "", "", errors.WithStackTrace(err)
	}

	return filepath.ToSlash(workingDir), filepath.ToSlash(downloadDir), nil
}

// Get the default IAM assume role session name.
func GetDefaultIAMAssumeRoleSessionName() string {
	return fmt.Sprintf("terragrunt-%d", time.Now().UTC().UnixNano())
}

// Create a new TerragruntOptions object with reasonable defaults for test usage
func NewTerragruntOptionsForTest(terragruntConfigPath string) (*TerragruntOptions, error) {
	opts, err := NewTerragruntOptions(terragruntConfigPath)

	if err != nil {
		logger := util.CreateLogEntry("", util.GetDefaultLogLevel())
		logger.Errorf("%v\n", errors.WithStackTrace(err))
		return nil, err
	}

	opts.NonInteractive = true
	opts.Logger = util.CreateLogEntry("", logrus.DebugLevel)
	opts.LogLevel = logrus.DebugLevel

	return opts, nil
}

// Create a copy of this TerragruntOptions, but with different values for the given variables. This is useful for
// creating a TerragruntOptions that behaves the same way, but is used for a Terraform module in a different folder.
func (terragruntOptions *TerragruntOptions) Clone(terragruntConfigPath string) *TerragruntOptions {
	workingDir := filepath.Dir(terragruntConfigPath)

	// Note that we clone lists and maps below as TerragruntOptions may be used and modified concurrently in the code
	// during xxx-all commands (e.g., apply-all, plan-all). See https://github.com/gruntwork-io/terragrunt/issues/367
	// for more info.
	return &TerragruntOptions{
		TerragruntConfigPath:           terragruntConfigPath,
		OriginalTerragruntConfigPath:   terragruntOptions.OriginalTerragruntConfigPath,
		TerraformPath:                  terragruntOptions.TerraformPath,
		OriginalTerraformCommand:       terragruntOptions.OriginalTerraformCommand,
		TerraformCommand:               terragruntOptions.TerraformCommand,
		TerraformVersion:               terragruntOptions.TerraformVersion,
		TerragruntVersion:              terragruntOptions.TerragruntVersion,
		AutoInit:                       terragruntOptions.AutoInit,
		RunAllAutoApprove:              terragruntOptions.RunAllAutoApprove,
		NonInteractive:                 terragruntOptions.NonInteractive,
		TerraformCliArgs:               util.CloneStringList(terragruntOptions.TerraformCliArgs),
		WorkingDir:                     workingDir,
		Logger:                         util.CreateLogEntryWithWriter(terragruntOptions.ErrWriter, workingDir, terragruntOptions.LogLevel, terragruntOptions.Logger.Logger.Hooks),
		LogLevel:                       terragruntOptions.LogLevel,
		ValidateStrict:                 terragruntOptions.ValidateStrict,
		Env:                            util.CloneStringMap(terragruntOptions.Env),
		Source:                         terragruntOptions.Source,
		SourceMap:                      terragruntOptions.SourceMap,
		SourceUpdate:                   terragruntOptions.SourceUpdate,
		DownloadDir:                    terragruntOptions.DownloadDir,
		Debug:                          terragruntOptions.Debug,
		OriginalIAMRoleOptions:         terragruntOptions.OriginalIAMRoleOptions,
		IAMRoleOptions:                 terragruntOptions.IAMRoleOptions,
		IgnoreDependencyErrors:         terragruntOptions.IgnoreDependencyErrors,
		IgnoreDependencyOrder:          terragruntOptions.IgnoreDependencyOrder,
		IgnoreExternalDependencies:     terragruntOptions.IgnoreExternalDependencies,
		IncludeExternalDependencies:    terragruntOptions.IncludeExternalDependencies,
		Writer:                         terragruntOptions.Writer,
		ErrWriter:                      terragruntOptions.ErrWriter,
		MaxFoldersToCheck:              terragruntOptions.MaxFoldersToCheck,
		AutoRetry:                      terragruntOptions.AutoRetry,
		RetryMaxAttempts:               terragruntOptions.RetryMaxAttempts,
		RetrySleepIntervalSec:          terragruntOptions.RetrySleepIntervalSec,
		RetryableErrors:                util.CloneStringList(terragruntOptions.RetryableErrors),
		ExcludeDirs:                    terragruntOptions.ExcludeDirs,
		IncludeDirs:                    terragruntOptions.IncludeDirs,
		ModulesThatInclude:             terragruntOptions.ModulesThatInclude,
		Parallelism:                    terragruntOptions.Parallelism,
		StrictInclude:                  terragruntOptions.StrictInclude,
		RunTerragrunt:                  terragruntOptions.RunTerragrunt,
		AwsProviderPatchOverrides:      terragruntOptions.AwsProviderPatchOverrides,
		HclFile:                        terragruntOptions.HclFile,
		JSONOut:                        terragruntOptions.JSONOut,
		Check:                          terragruntOptions.Check,
		CheckDependentModules:          terragruntOptions.CheckDependentModules,
		FetchDependencyOutputFromState: terragruntOptions.FetchDependencyOutputFromState,
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

// Inserts the given argsToInsert after the terraform command argument, but before the remaining args
func (terragruntOptions *TerragruntOptions) InsertTerraformCliArgs(argsToInsert ...string) {
	planFile, restArgs := extractPlanFile(argsToInsert)

	commandLength := 1
	if util.ListContainsElement(TERRAFORM_COMMANDS_WITH_SUBCOMMAND, terragruntOptions.TerraformCliArgs[0]) {
		// Since these terraform commands require subcommands which may not always be properly passed by the user,
		// using util.Min to return the minimum to avoid potential out of bounds slice errors.
		commandLength = util.Min(2, len(terragruntOptions.TerraformCliArgs))
	}

	// Options must be inserted after command but before the other args
	// command is either 1 word or 2 words
	var args []string
	args = append(args, terragruntOptions.TerraformCliArgs[:commandLength]...)
	args = append(args, restArgs...)
	args = append(args, terragruntOptions.TerraformCliArgs[commandLength:]...)

	// check if planfile was extracted
	if planFile != nil {
		args = append(args, *planFile)
	}

	terragruntOptions.TerraformCliArgs = args
}

// Appends the given argsToAppend after the current TerraformCliArgs
func (terragruntOptions *TerragruntOptions) AppendTerraformCliArgs(argsToAppend ...string) {
	terragruntOptions.TerraformCliArgs = append(terragruntOptions.TerraformCliArgs, argsToAppend...)
}

// TerraformDataDir returns Terraform data directory (.terraform by default, overridden by $TF_DATA_DIR envvar)
func (terragruntOptions *TerragruntOptions) TerraformDataDir() string {
	if tfDataDir, ok := terragruntOptions.Env["TF_DATA_DIR"]; ok {
		return tfDataDir
	}
	return DefaultTFDataDir
}

// DataDir returns the Terraform data directory prepended with the working directory path,
// or just the Terraform data directory if it is an absolute path.
func (terragruntOptions *TerragruntOptions) DataDir() string {
	tfDataDir := terragruntOptions.TerraformDataDir()
	if filepath.IsAbs(tfDataDir) {
		return tfDataDir
	}
	return util.JoinPath(terragruntOptions.WorkingDir, tfDataDir)
}

// Custom error types

var RunTerragruntCommandNotSet = fmt.Errorf("The RunTerragrunt option has not been set on this TerragruntOptions object")
