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

func RunShellCommandAndGetOutput(command string, args ... string) (string, error) {
	cmd := exec.Command(command, args...)

	cmd.Stdin = os.Stdin

	bytes, err := cmd.CombinedOutput()
	if err != nil {
		return "", err
	} else {
		return string(bytes), nil
	}
}
