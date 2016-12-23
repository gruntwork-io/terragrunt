package shell

import (
	"bytes"
	"github.com/gruntwork-io/terragrunt/errors"
	"github.com/gruntwork-io/terragrunt/options"
	"os"
	"os/exec"
	"strings"
)

// Run the specified shell command with the specified arguments. Connect the command's stdin, stdout, and stderr to
// the currently running app.
func RunShellCommand(terragruntOptions *options.TerragruntOptions, command string, args ...string) error {
	terragruntOptions.Logger.Printf("Running command: %s %s", command, strings.Join(args, " "))

	cmd := exec.Command(command, args...)

	// TODO: consider adding prefix from terragruntOptions logger to stdout and stderr
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	cmd.Dir = terragruntOptions.WorkingDir

	return errors.WithStackTrace(cmd.Run())
}

// Run the specified shell command with the specified arguments. Connect the command's stdin to
// the current running app, and return the fully read stdout and stderr streams as strings.
func GetShellOutput(terragruntOptions *options.TerragruntOptions, command string, args ...string) (string, string, error) {
	terragruntOptions.Logger.Printf("Running command: %s %s", command, strings.Join(args, " "))

	cmd := exec.Command(command, args...)

	var stdout, stderr bytes.Buffer

	cmd.Stdin = os.Stdin
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := errors.WithStackTrace(cmd.Run())

	return stdout.String(), stderr.String(), err
}
