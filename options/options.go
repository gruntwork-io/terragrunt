package options

import (
	"fmt"
	"io"
	"log"
	"math"
	"os"
	"path/filepath"
	"time"

	"github.com/gruntwork-io/terragrunt/errors"
	"github.com/gruntwork-io/terragrunt/util"
	"github.com/hashicorp/go-version"
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

// TerragruntOptions represents options that configure the behavior of the Terragrunt program
type TerragruntOptions struct {
	// Location of the Terragrunt config file
	TerragruntConfigPath string

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

	// CLI args that are intended for Terraform (i.e. all the CLI args except the --terragrunt ones)
	TerraformCliArgs []string

	// The working directory in which to run Terraform
	WorkingDir string

	// The logger to use for all logging
	Logger *log.Logger

	// Environment variables at runtime
	Env map[string]string

	// Download Terraform configurations from the specified source location into a temporary folder and run
	// Terraform in that temporary folder
	Source string

	// If set to true, delete the contents of the temporary folder before downloading Terraform source code into it
	SourceUpdate bool

	// Download Terraform configurations specified in the Source parameter into this folder
	DownloadDir string

	// The ARN of an IAM Role to assume before running Terraform
	IamRole string

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

	// Whether we should automatically run terraform init if necessary when executing other commands
	AutoRetry bool

	// Maximum number of times to retry errors matching RetryableErrors
	MaxRetryAttempts int

	// Sleep is the duration in seconds to wait before retrying
	Sleep time.Duration

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
}

// Create a new TerragruntOptions object with reasonable defaults for real usage
func NewTerragruntOptions(terragruntConfigPath string) (*TerragruntOptions, error) {
	logger := util.CreateLogger("")

	workingDir, downloadDir, err := DefaultWorkingAndDownloadDirs(terragruntConfigPath)
	if err != nil {
		return nil, errors.WithStackTrace(err)
	}

	return &TerragruntOptions{
		TerragruntConfigPath:        terragruntConfigPath,
		TerraformPath:               TERRAFORM_DEFAULT_PATH,
		OriginalTerraformCommand:    "",
		TerraformCommand:            "",
		AutoInit:                    true,
		NonInteractive:              false,
		TerraformCliArgs:            []string{},
		WorkingDir:                  workingDir,
		Logger:                      logger,
		Env:                         map[string]string{},
		Source:                      "",
		SourceUpdate:                false,
		DownloadDir:                 downloadDir,
		IgnoreDependencyErrors:      false,
		IgnoreDependencyOrder:       false,
		IgnoreExternalDependencies:  false,
		IncludeExternalDependencies: false,
		Writer:                      os.Stdout,
		ErrWriter:                   os.Stderr,
		MaxFoldersToCheck:           DEFAULT_MAX_FOLDERS_TO_CHECK,
		AutoRetry:                   true,
		MaxRetryAttempts:            DEFAULT_MAX_RETRY_ATTEMPTS,
		Sleep:                       DEFAULT_SLEEP,
		RetryableErrors:             util.CloneStringList(DEFAULT_RETRYABLE_ERRORS),
		ExcludeDirs:                 []string{},
		IncludeDirs:                 []string{},
		StrictInclude:               false,
		Parallelism:                 DEFAULT_PARALLELISM,
		Check:                       false,
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

// Create a new TerragruntOptions object with reasonable defaults for test usage
func NewTerragruntOptionsForTest(terragruntConfigPath string) (*TerragruntOptions, error) {
	opts, err := NewTerragruntOptions(terragruntConfigPath)

	if err != nil {
		logger := util.CreateLogger("")
		logger.Printf("error: %v\n", errors.WithStackTrace(err))
		return nil, err
	}

	opts.NonInteractive = true

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
		TerragruntConfigPath:        terragruntConfigPath,
		TerraformPath:               terragruntOptions.TerraformPath,
		OriginalTerraformCommand:    terragruntOptions.OriginalTerraformCommand,
		TerraformCommand:            terragruntOptions.TerraformCommand,
		TerraformVersion:            terragruntOptions.TerraformVersion,
		TerragruntVersion:           terragruntOptions.TerragruntVersion,
		AutoInit:                    terragruntOptions.AutoInit,
		NonInteractive:              terragruntOptions.NonInteractive,
		TerraformCliArgs:            util.CloneStringList(terragruntOptions.TerraformCliArgs),
		WorkingDir:                  workingDir,
		Logger:                      util.CreateLoggerWithWriter(terragruntOptions.ErrWriter, workingDir),
		Env:                         util.CloneStringMap(terragruntOptions.Env),
		Source:                      terragruntOptions.Source,
		SourceUpdate:                terragruntOptions.SourceUpdate,
		DownloadDir:                 terragruntOptions.DownloadDir,
		IamRole:                     terragruntOptions.IamRole,
		IgnoreDependencyErrors:      terragruntOptions.IgnoreDependencyErrors,
		IgnoreDependencyOrder:       terragruntOptions.IgnoreDependencyOrder,
		IgnoreExternalDependencies:  terragruntOptions.IgnoreExternalDependencies,
		IncludeExternalDependencies: terragruntOptions.IncludeExternalDependencies,
		Writer:                      terragruntOptions.Writer,
		ErrWriter:                   terragruntOptions.ErrWriter,
		MaxFoldersToCheck:           terragruntOptions.MaxFoldersToCheck,
		AutoRetry:                   terragruntOptions.AutoRetry,
		MaxRetryAttempts:            terragruntOptions.MaxRetryAttempts,
		Sleep:                       terragruntOptions.Sleep,
		RetryableErrors:             util.CloneStringList(terragruntOptions.RetryableErrors),
		ExcludeDirs:                 terragruntOptions.ExcludeDirs,
		IncludeDirs:                 terragruntOptions.IncludeDirs,
		Parallelism:                 terragruntOptions.Parallelism,
		StrictInclude:               terragruntOptions.StrictInclude,
		RunTerragrunt:               terragruntOptions.RunTerragrunt,
		AwsProviderPatchOverrides:   terragruntOptions.AwsProviderPatchOverrides,
	}
}

// Inserts the given argsToInsert after the terraform command argument, but before the remaining args
func (terragruntOptions *TerragruntOptions) InsertTerraformCliArgs(argsToInsert ...string) {

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
	args = append(args, argsToInsert...)
	args = append(args, terragruntOptions.TerraformCliArgs[commandLength:]...)
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
