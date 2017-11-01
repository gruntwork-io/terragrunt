package options

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"runtime"

	"github.com/gruntwork-io/terragrunt/errors"
	"github.com/gruntwork-io/terragrunt/util"
	version "github.com/hashicorp/go-version"
)

var TERRAFORM_COMMANDS_WITH_SUBCOMMAND = []string{
	"debug",
	"force-unlock",
	"state",
}

// TerragruntOptions represents options that configure the behavior of the Terragrunt program
type TerragruntOptions struct {
	// Location of the Terragrunt config file
	TerragruntConfigPath string

	// Location of the terraform binary
	TerraformPath string

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

	// If you want stdout to go somewhere other than os.stdout
	Writer io.Writer

	// If you want stderr to go somewhere other than os.stderr
	ErrWriter io.Writer

	// A command that can be used to run Terragrunt with the given options. This is useful for running Terragrunt
	// multiple times (e.g. when spinning up a stack of Terraform modules). The actual command is normally defined
	// in the cli package, which depends on almost all other packages, so we declare it here so that other
	// packages can use the command without a direct reference back to the cli package (which would create a
	// circular dependency).
	RunTerragrunt func(*TerragruntOptions) error
}

// Create a new TerragruntOptions object with reasonable defaults for real usage
func NewTerragruntOptions(terragruntConfigPath string) *TerragruntOptions {
	workingDir := filepath.Dir(terragruntConfigPath)

	user, err := user.Current()
	if err != nil {
		return err
	}

	downloadDir := filepath.Join(user.HomeDir, ".terragrunt")
	// On some versions of Windows, the default temp dir is a fairly long path (e.g. C:/Users/JONDOE~1/AppData/Local/Temp/2/).
	// This is a problem because Windows also limits path lengths to 260 characters, and with nested folders and hashed folder names
	// (e.g. from running terraform get), you can hit that limit pretty quickly. Therefore, we try to set the temporary download
	// folder to something slightly shorter, but still reasonable.
	if runtime.GOOS == "windows" {
		downloadDir = `C:\\Windows\\Temp\\terragrunt`
	}

	return &TerragruntOptions{
		TerragruntConfigPath:   terragruntConfigPath,
		TerraformPath:          "terraform",
		AutoInit:               true,
		NonInteractive:         false,
		TerraformCliArgs:       []string{},
		WorkingDir:             workingDir,
		Logger:                 util.CreateLogger(""),
		Env:                    map[string]string{},
		Source:                 "",
		SourceUpdate:           false,
		DownloadDir:            downloadDir,
		IgnoreDependencyErrors: false,
		Writer:                 os.Stdout,
		ErrWriter:              os.Stderr,
		RunTerragrunt: func(terragruntOptions *TerragruntOptions) error {
			return errors.WithStackTrace(RunTerragruntCommandNotSet)
		},
	}
}

// Create a new TerragruntOptions object with reasonable defaults for test usage
func NewTerragruntOptionsForTest(terragruntConfigPath string) *TerragruntOptions {
	opts := NewTerragruntOptions(terragruntConfigPath)

	opts.NonInteractive = true

	return opts
}

// Create a copy of this TerragruntOptions, but with different values for the given variables. This is useful for
// creating a TerragruntOptions that behaves the same way, but is used for a Terraform module in a different folder.
func (terragruntOptions *TerragruntOptions) Clone(terragruntConfigPath string) *TerragruntOptions {
	workingDir := filepath.Dir(terragruntConfigPath)

	return &TerragruntOptions{
		TerragruntConfigPath:   terragruntConfigPath,
		TerraformPath:          terragruntOptions.TerraformPath,
		TerraformVersion:       terragruntOptions.TerraformVersion,
		AutoInit:               terragruntOptions.AutoInit,
		NonInteractive:         terragruntOptions.NonInteractive,
		TerraformCliArgs:       terragruntOptions.TerraformCliArgs,
		WorkingDir:             workingDir,
		Logger:                 util.CreateLoggerWithWriter(terragruntOptions.ErrWriter, workingDir),
		Env:                    terragruntOptions.Env,
		Source:                 terragruntOptions.Source,
		SourceUpdate:           terragruntOptions.SourceUpdate,
		DownloadDir:            terragruntOptions.DownloadDir,
		IamRole:                terragruntOptions.IamRole,
		IgnoreDependencyErrors: terragruntOptions.IgnoreDependencyErrors,
		Writer:                 terragruntOptions.Writer,
		ErrWriter:              terragruntOptions.ErrWriter,
		RunTerragrunt:          terragruntOptions.RunTerragrunt,
	}
}

// Inserts the given argsToInsert after the terraform command argument, but before the remaining args
func (terragruntOptions *TerragruntOptions) InsertTerraformCliArgs(argsToInsert ...string) {

	commandLength := 1
	if util.ListContainsElement(TERRAFORM_COMMANDS_WITH_SUBCOMMAND, terragruntOptions.TerraformCliArgs[0]) {
		commandLength = 2
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

// Custom error types

var RunTerragruntCommandNotSet = fmt.Errorf("The RunTerragrunt option has not been set on this TerragruntOptions object")
