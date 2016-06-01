package shell

import (
	"os"
	"os/exec"
	"strings"
	"github.com/gruntwork-io/terragrunt/util"
	"github.com/gruntwork-io/terragrunt/errors"
)

// Run the specified shell command with the specified arguments. Connect the command's stdin, stdout, and stderr to
// the currently running app.
func RunShellCommand(command string, args ... string) error {
	util.Logger.Printf("Running command: %s %s", command, strings.Join(args, " "))

	cmd := exec.Command(command, args...)

	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	return errors.WithStackTrace(cmd.Run())
}