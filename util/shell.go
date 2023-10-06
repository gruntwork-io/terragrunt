package util

import "os/exec"

// IsCommandExecutable - returns true if a command can be executed without errors.
func IsCommandExecutable(command string, args ...string) bool {
	cmd := exec.Command(command, args...)
	cmd.Stdin = nil
	cmd.Stdout = nil
	cmd.Stderr = nil

	if err := cmd.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return exitErr.ExitCode() == 0
		}
		return false
	}
	return true
}
