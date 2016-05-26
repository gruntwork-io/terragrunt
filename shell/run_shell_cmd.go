package shell

import (
	"os"
	"os/exec"
)

func RunShellCommand(command string, args ... string) error {
	cmd := exec.Command(command, args...)

	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	return cmd.Run()
}
