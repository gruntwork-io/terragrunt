package shell

import (
	"os"
	"os/exec"
	"strings"
	"github.com/gruntwork-io/terragrunt/util"
)

func RunShellCommand(command string, args ... string) error {
	util.Logger.Printf("Running command: %s %s", command, strings.Join(args, " "))

	cmd := exec.Command(command, args...)

	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	return cmd.Run()
}