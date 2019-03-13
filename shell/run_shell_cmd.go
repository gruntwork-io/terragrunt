package shell

import (
	"bytes"
	"fmt"
	"io"
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

// RunTerraformCommand Run the given Terraform command
func RunTerraformCommand(terragruntOptions *options.TerragruntOptions, args ...string) error {
	_, err := RunShellCommandWithOutput(terragruntOptions, "", terragruntOptions.TerraformPath, args...)
	return err
}

// RunShellCommand Run the given shell command
func RunShellCommand(terragruntOptions *options.TerragruntOptions, command string, args ...string) error {
	_, err := RunShellCommandWithOutput(terragruntOptions, "", command, args...)
	return err
}

// RunTerraformCommandWithOutput Run the given Terraform command, writing its stdout/stderr to the terminal AND returning stdout/stderr to this
// method's caller
func RunTerraformCommandWithOutput(terragruntOptions *options.TerragruntOptions, args ...string) (*CmdOutput, error) {
	return RunShellCommandWithOutput(terragruntOptions, "", terragruntOptions.TerraformPath, args...)
}

// Run the specified shell command with the specified arguments. Connect the command's stdin, stdout, and stderr to
// the currently running app. The command can be executed in a custom working directory by using the parameter `workingDir`. Terragrunt working directory will be assumed if empty empty.
func RunShellCommandWithOutput(terragruntOptions *options.TerragruntOptions, workingDir string, command string, args ...string) (*CmdOutput, error) {
	terragruntOptions.Logger.Printf("Running command: %s %s", command, strings.Join(args, " "))

	var stdoutBuf bytes.Buffer
	var stderrBuf bytes.Buffer

	/**
	 * perform Interpolation on arguments, copy the result to new array and pass this
	 * to exec.Commnand
	 */
	include := terragruntOptions.GetIncludeConfig()
	argsParsed := make([]string, len(args))
	for i := 0; i < len(args); i++ {
		value, err := ResolveTerragruntConfigString(args[i], include, terragruntOptions)
		if err != nil {
			return nil, errors.WithStackTrace(err)
		}
		argsParsed[i] = value
	}
	cmd := exec.Command(command, argsParsed...)
	terragruntOptions.Logger.Printf("Running command: %s %s", command, strings.Join(argsParsed, " "))

	// TODO: consider adding prefix from terragruntOptions logger to stdout and stderr
	cmd.Stdin = os.Stdin
	pEnvVars, errEnv := toEnvVarsList(terragruntOptions.Env, terragruntOptions)
	if errEnv != nil {
		return nil, errors.WithStackTrace(errEnv)
	}
	cmd.Env = pEnvVars

	var errWriter = terragruntOptions.ErrWriter
	var outWriter = terragruntOptions.Writer
	// Terragrunt can run some commands (such as terraform remote config) before running the actual terraform
	// command requested by the user. The output of these other commands should not end up on stdout as this
	// breaks scripts relying on terraform's output.
	// FIXME: do we have to inetrpolate TerraformCliArgs and compare with argsParsed from above?
	if !reflect.DeepEqual(terragruntOptions.TerraformCliArgs, args) {
		outWriter = terragruntOptions.ErrWriter
	}

	if workingDir == "" {
		cmd.Dir = terragruntOptions.WorkingDir
	} else {
		cmd.Dir = workingDir
	}
	// Inspired by https://blog.kowalczyk.info/article/wOYk/advanced-command-execution-in-go-with-osexec.html
	cmd.Stderr = io.MultiWriter(errWriter, &stderrBuf)
	cmd.Stdout = io.MultiWriter(outWriter, &stdoutBuf)

	if err := cmd.Start(); err != nil {
		// bad path, binary not executable, &c
		return nil, errors.WithStackTrace(err)
	}

	cmdChannel := make(chan error)
	signalChannel := NewSignalsForwarder(forwardSignals, cmd, terragruntOptions.Logger, cmdChannel)
	defer signalChannel.Close()

	err := cmd.Wait()
	cmdChannel <- err

	cmdOutput := CmdOutput{
		Stdout: stdoutBuf.String(),
		Stderr: stderrBuf.String(),
	}

	return &cmdOutput, errors.WithStackTrace(err)
}

func toEnvVarsList(envVarsAsMap map[string]string, terragruntOptions *options.TerragruntOptions) ([]string, error) {
	envVarsAsList := []string{}
	include := terragruntOptions.GetIncludeConfig()
	for key, value := range envVarsAsMap {
		pvalue, err := ResolveTerragruntConfigString(value, include, terragruntOptions)
		if err != nil {
			return nil, err
		}
		envVarsAsList = append(envVarsAsList, fmt.Sprintf("%s=%s", key, pvalue))
	}
	return envVarsAsList, nil
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

type CmdOutput struct {
	Stdout string
	Stderr string
}
