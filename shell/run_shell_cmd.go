package shell

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"reflect"
	"strings"
	"syscall"
	"time"

	"github.com/gruntwork-io/terragrunt/errors"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/util"
	"github.com/hashicorp/go-multierror"
	"github.com/sirupsen/logrus"
)

// The signal can be sent to the main process (only `terragrunt`) as well as the process group (`terragrunt` and `terraform`), for example:
// kill -INT <pid>  # sends SIGINT only to the main process
// kill -INT -<pid> # sends SIGINT to the process group
// Since we cannot know how the signal is sent, we should give `terraform` time to gracefully exit if it receives the signal directly from the shell, to avoid sending the second interrupt signal to `terraform`.
const signalForwardingDelay = time.Second * 30

// Commands that implement a REPL need a pseudo TTY when run as a subprocess in order for the readline properties to be
// preserved. This is a list of terraform commands that have this property, which is used to determine if terragrunt
// should allocate a ptty when running that terraform command.
var terraformCommandsThatNeedPty = []string{
	"console",
}

// Run the given Terraform command
func RunTerraformCommand(terragruntOptions *options.TerragruntOptions, args ...string) error {
	_, err := RunShellCommandWithOutput(terragruntOptions, "", false, isTerraformCommandThatNeedsPty(args[0]), terragruntOptions.TerraformPath, args...)
	return err
}

// Run the given shell command
func RunShellCommand(terragruntOptions *options.TerragruntOptions, command string, args ...string) error {
	_, err := RunShellCommandWithOutput(terragruntOptions, "", false, false, command, args...)
	return err
}

// Run the given Terraform command, writing its stdout/stderr to the terminal AND returning stdout/stderr to this
// method's caller
func RunTerraformCommandWithOutput(terragruntOptions *options.TerragruntOptions, args ...string) (*CmdOutput, error) {
	needPty := false
	if len(args) > 0 {
		needPty = isTerraformCommandThatNeedsPty(args[0])
	}
	return RunShellCommandWithOutput(terragruntOptions, "", false, needPty, terragruntOptions.TerraformPath, args...)
}

// Run the specified shell command with the specified arguments. Connect the command's stdin, stdout, and stderr to
// the currently running app. The command can be executed in a custom working directory by using the parameter
// `workingDir`. Terragrunt working directory will be assumed if empty string.
func RunShellCommandWithOutput(
	terragruntOptions *options.TerragruntOptions,
	workingDir string,
	suppressStdout bool,
	allocatePseudoTty bool,
	command string,
	args ...string,
) (*CmdOutput, error) {
	terragruntOptions.Logger.Debugf("Running command: %s %s", command, strings.Join(args, " "))
	if suppressStdout {
		terragruntOptions.Logger.Debugf("Command output will be suppressed.")
	}

	var stdoutBuf bytes.Buffer
	var stderrBuf bytes.Buffer

	cmd := exec.Command(command, args...)

	// TODO: consider adding prefix from terragruntOptions logger to stdout and stderr
	cmd.Env = toEnvVarsList(terragruntOptions.Env)

	var errWriter = terragruntOptions.ErrWriter
	var outWriter = terragruntOptions.Writer
	var prefix = ""
	if terragruntOptions.IncludeModulePrefix {
		prefix = terragruntOptions.OutputPrefix
	}
	// Terragrunt can run some commands (such as terraform remote config) before running the actual terraform
	// command requested by the user. The output of these other commands should not end up on stdout as this
	// breaks scripts relying on terraform's output.
	if !reflect.DeepEqual(terragruntOptions.TerraformCliArgs, args) {
		outWriter = terragruntOptions.ErrWriter
	}

	if workingDir == "" {
		cmd.Dir = terragruntOptions.WorkingDir
	} else {
		cmd.Dir = workingDir
	}

	// Inspired by https://blog.kowalczyk.info/article/wOYk/advanced-command-execution-in-go-with-osexec.html
	cmdStderr := io.MultiWriter(withPrefix(errWriter, prefix), &stderrBuf)
	var cmdStdout io.Writer
	if !suppressStdout {
		cmdStdout = io.MultiWriter(withPrefix(outWriter, prefix), &stdoutBuf)
	} else {
		cmdStdout = io.MultiWriter(&stdoutBuf)
	}

	// If we need to allocate a ptty for the command, route through the ptty routine. Otherwise, directly call the
	// command.
	if allocatePseudoTty {
		if err := runCommandWithPTTY(terragruntOptions, cmd, cmdStdout, cmdStderr); err != nil {
			return nil, err
		}
	} else {
		cmd.Stdin = os.Stdin
		cmd.Stdout = cmdStdout
		cmd.Stderr = cmdStderr
		if err := cmd.Start(); err != nil {
			// bad path, binary not executable, &c
			return nil, errors.WithStackTrace(err)
		}
	}

	// Make sure to forward signals to the subcommand.
	cmdChannel := make(chan error) // used for closing the signals forwarder goroutine
	signalChannel := NewSignalsForwarder(forwardSignals, cmd, terragruntOptions.Logger, cmdChannel)
	defer signalChannel.Close()

	err := cmd.Wait()
	cmdChannel <- err

	cmdOutput := CmdOutput{
		Stdout: stdoutBuf.String(),
		Stderr: stderrBuf.String(),
	}

	if err != nil {
		err = ProcessExecutionError{
			Err:        err,
			StdOut:     stdoutBuf.String(),
			Stderr:     stderrBuf.String(),
			WorkingDir: cmd.Dir,
		}
	}

	return &cmdOutput, errors.WithStackTrace(err)
}

func toEnvVarsList(envVarsAsMap map[string]string) []string {
	envVarsAsList := []string{}
	for key, value := range envVarsAsMap {
		envVarsAsList = append(envVarsAsList, fmt.Sprintf("%s=%s", key, value))
	}
	return envVarsAsList
}

// isTerraformCommandThatNeedsPty returns true if the sub command of terraform we are running requires a pty.
func isTerraformCommandThatNeedsPty(command string) bool {
	return util.ListContainsElement(terraformCommandsThatNeedPty, command)
}

// Return the exit code of a command. If the error does not implement errors.IErrorCode or is not an exec.ExitError
// or *multierror.Error type, the error is returned.
func GetExitCode(err error) (int, error) {
	if exiterr, ok := errors.Unwrap(err).(errors.IErrorCode); ok {
		return exiterr.ExitStatus()
	}

	if exiterr, ok := errors.Unwrap(err).(*exec.ExitError); ok {
		status := exiterr.Sys().(syscall.WaitStatus)
		return status.ExitStatus(), nil
	}

	if exiterr, ok := errors.Unwrap(err).(*multierror.Error); ok {
		for _, err := range exiterr.Errors {
			exitCode, exitCodeErr := GetExitCode(err)
			if exitCodeErr == nil {
				return exitCode, nil
			}
		}
	}

	return 0, err
}

func withPrefix(writer io.Writer, prefix string) io.Writer {
	if prefix == "" {
		return writer
	}

	return util.PrefixedWriter(writer, prefix)
}

type SignalsForwarder chan os.Signal

// Forwards signals to a command, waiting for the command to finish.
func NewSignalsForwarder(signals []os.Signal, c *exec.Cmd, logger *logrus.Entry, cmdChannel chan error) SignalsForwarder {
	signalChannel := make(chan os.Signal, 1)
	signal.Notify(signalChannel, signals...)

	go func() {
		for {
			select {
			case s := <-signalChannel:
				logger.Debugf("%s signal received. Gracefully shutting down... (it can take up to %v)", strings.Title(s.String()), signalForwardingDelay)

				select {
				case <-time.After(signalForwardingDelay):
					logger.Debugf("Forward signal %v to terraform.", s)
					err := c.Process.Signal(s)
					if err != nil {
						logger.Errorf("Error forwarding signal: %v", err)
					}
				case <-cmdChannel:
					return
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

// GitTopLevelDir - fetch git repository path from passed directory
func GitTopLevelDir(terragruntOptions *options.TerragruntOptions, path string) (string, error) {
	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}
	opts, err := options.NewTerragruntOptions(path)
	if err != nil {
		return "", err
	}
	opts.Env = terragruntOptions.Env
	opts.Writer = &stdout
	opts.ErrWriter = &stderr
	cmd, err := RunShellCommandWithOutput(opts, path, true, false, "git", "rev-parse", "--show-toplevel")
	terragruntOptions.Logger.Debugf("git show-toplevel result: \n%v\n%v\n", (string)(stdout.Bytes()), (string)(stderr.Bytes()))
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(cmd.Stdout), nil
}

// ProcessExecutionError - error returned when a command fails, contains StdOut and StdErr
type ProcessExecutionError struct {
	Err        error
	StdOut     string
	Stderr     string
	WorkingDir string
}

func (err ProcessExecutionError) Error() string {
	// Include in error message the working directory where the command was run, so it's easier for the user to
	return fmt.Sprintf("[%s] %s", err.WorkingDir, err.Err.Error())
}

func (err ProcessExecutionError) ExitStatus() (int, error) {
	return GetExitCode(err.Err)
}
