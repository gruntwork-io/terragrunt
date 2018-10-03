package shell

import (
	"bufio"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"reflect"
	"strings"
	"syscall"

	"github.com/gruntwork-io/terragrunt/errors"
	"github.com/gruntwork-io/terragrunt/options"
)

// Run the given Terraform command
func RunTerraformCommand(terragruntOptions *options.TerragruntOptions, args ...string) error {
	_, err := RunShellCommandWithOutput(terragruntOptions, terragruntOptions.TerraformPath, args...)
	return err
}

// Run the given shell command
func RunShellCommand(terragruntOptions *options.TerragruntOptions, command string, args ...string) error {
	_, err := RunShellCommandWithOutput(terragruntOptions, command, args...)
	return err
}

// Run the given Terraform command
func RunTerraformCommandWithOutput(terragruntOptions *options.TerragruntOptions, args ...string) (string, error) {
	return RunShellCommandWithOutput(terragruntOptions, terragruntOptions.TerraformPath, args...)
}

// Run the given Terraform command and return the stdout as a string
func RunTerraformCommandAndCaptureOutput(terragruntOptions *options.TerragruntOptions, args ...string) (string, error) {
	return RunShellCommandAndCaptureOutput(terragruntOptions, terragruntOptions.TerraformPath, args...)
}

// Run the specified shell command with the specified arguments. Connect the command's stdin, stdout, and stderr to
// the currently running app.
func RunShellCommandWithOutput(terragruntOptions *options.TerragruntOptions, command string, args ...string) (string, error) {
	terragruntOptions.Logger.Printf("Running command: %s %s", command, strings.Join(args, " "))

	var outToErr bool = false

	cmd := exec.Command(command, args...)

	stdoutIn, err := cmd.StdoutPipe()
	if err != nil {
		return "", errors.WithStackTrace(err)
	}

	stderrIn, err := cmd.StderrPipe()
	if err != nil {
		return "", errors.WithStackTrace(err)
	}

	// TODO: consider adding prefix from terragruntOptions logger to stdout and stderr
	cmd.Stdin = os.Stdin
	cmd.Env = toEnvVarsList(terragruntOptions.Env)

	// Terragrunt can run some commands (such as terraform remote config) before running the actual terraform
	// command requested by the user. The output of these other commands should not end up on stdout as this
	// breaks scripts relying on terraform's output.
	if !reflect.DeepEqual(terragruntOptions.TerraformCliArgs, args) {
		outToErr = true
	}

	cmd.Dir = terragruntOptions.WorkingDir

	if err := cmd.Start(); err != nil {
		// bad path, binary not executable, &c
		return "", errors.WithStackTrace(err)
	}

	output, err := readStdoutAndStderr(stdoutIn, stderrIn, terragruntOptions, outToErr)
	if err != nil {
		return "", errors.WithStackTrace(err)
	}

	cmdChannel := make(chan error)
	signalChannel := NewSignalsForwarder(forwardSignals, cmd, terragruntOptions.Logger, cmdChannel)
	defer signalChannel.Close()

	err = cmd.Wait()
	cmdChannel <- err

	terragruntOptions.Logger.Println(fmt.Sprintf("And the output is: %v", output))

	return output, errors.WithStackTrace(err)
}

func toEnvVarsList(envVarsAsMap map[string]string) []string {
	envVarsAsList := []string{}
	for key, value := range envVarsAsMap {
		envVarsAsList = append(envVarsAsList, fmt.Sprintf("%s=%s", key, value))
	}
	return envVarsAsList
}

// Run the specified shell command with the specified arguments. Capture the command's stdout and return it as a
// string.
func RunShellCommandAndCaptureOutput(terragruntOptions *options.TerragruntOptions, command string, args ...string) (string, error) {
	stdout := ioutil.Discard

	terragruntOptionsCopy := terragruntOptions.Clone(terragruntOptions.TerragruntConfigPath)
	terragruntOptionsCopy.Writer = stdout
	terragruntOptionsCopy.ErrWriter = stdout

	return RunShellCommandWithOutput(terragruntOptionsCopy, command, args...)
}

// Return the exit code of a command. If the error does not implement errors.IErrorCode or is not an exec.ExitError
// or errors.MultiError type, the error is returned.
func GetExitCode(err error) (int, error) {
	if exiterr, ok := errors.Unwrap(err).(errors.IErrorCode); ok {
		return exiterr.ExitStatus()
	}

	if exiterr, ok := errors.Unwrap(err).(*exec.ExitError); ok {
		status := exiterr.Sys().(syscall.WaitStatus)
		return status.ExitStatus(), nil
	}

	if exiterr, ok := errors.Unwrap(err).(errors.MultiError); ok {
		for _, err := range exiterr.Errors {
			exitCode, exitCodeErr := GetExitCode(err)
			if exitCodeErr == nil {
				return exitCode, nil
			}
		}
	}

	return 0, err
}

type SignalsForwarder chan os.Signal

// Forwards signals to a command, waiting for the command to finish.
func NewSignalsForwarder(signals []os.Signal, c *exec.Cmd, logger *log.Logger, cmdChannel chan error) SignalsForwarder {
	signalChannel := make(chan os.Signal, 1)
	signal.Notify(signalChannel, signals...)

	go func() {
		for {
			select {
			case s := <-signalChannel:
				logger.Printf("Forward signal %v to terraform.", s)
				err := c.Process.Signal(s)
				if err != nil {
					logger.Printf("Error forwarding signal: %v", err)
				}
			case <-cmdChannel:
				return
			}
		}
	}()

	return signalChannel
}

func (signalChannel *SignalsForwarder) Close() error {
	signal.Stop(*signalChannel)
	*signalChannel <- nil
	close(*signalChannel)
	return nil
}

// This function captures stdout and stderr while still printing it to the stdout and stderr of this Go program
// outToErr flips std out to std err
func readStdoutAndStderr(stdout io.ReadCloser, stderr io.ReadCloser, terragruntOptions *options.TerragruntOptions, outToErr bool) (string, error) {
	allOutput := []string{}

	var errWriter = terragruntOptions.ErrWriter
	var outWriter = terragruntOptions.Writer

	if outToErr {
		outWriter = terragruntOptions.ErrWriter
	}

	stdoutScanner := bufio.NewScanner(stdout)
	stderrScanner := bufio.NewScanner(stderr)

	for {
		if stdoutScanner.Scan() {
			text := stdoutScanner.Text()
			fmt.Fprintln(outWriter, text)
			allOutput = append(allOutput, text)
		} else if stderrScanner.Scan() {
			text := stderrScanner.Text()
			fmt.Fprintln(errWriter, text)
			allOutput = append(allOutput, text)
		} else {
			break
		}
	}

	if err := stdoutScanner.Err(); err != nil {
		return "", err
	}

	if err := stderrScanner.Err(); err != nil {
		return "", err
	}

	return strings.Join(allOutput, "\n"), nil
}
