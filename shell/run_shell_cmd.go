package shell

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"golang.org/x/text/cases"
	"golang.org/x/text/language"

	"github.com/gruntwork-io/go-commons/collections"
	"github.com/gruntwork-io/go-commons/errors"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/terraform"
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

var terraformInitMutex sync.Mutex

// Run the given Terraform command
func RunTerraformCommand(terragruntOptions *options.TerragruntOptions, args ...string) error {
	needPTY, err := isTerraformCommandThatNeedsPty(args)
	if err != nil {
		return err
	}

	_, err = RunShellCommandWithOutput(terragruntOptions, "", false, needPTY, terragruntOptions.TerraformPath, args...)
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
	needPTY, err := isTerraformCommandThatNeedsPty(args)
	if err != nil {
		return nil, err
	}

	return RunShellCommandWithOutput(terragruntOptions, "", false, needPTY, terragruntOptions.TerraformPath, args...)
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
	// Terrafrom `init` command with the plugin cache directory is not guaranteed to be concurrency safe.
	// The provider installer's behavior in environments with multiple terraform init calls is undefined.
	// Thus, terraform `init` commands must be executed sequentially, even if `--terragrunt-parallelism` is greater than 1.
	if command == "terraform" && collections.ListContainsElement(args, "init") && terraform.IsPluginCacheUsed() {
		defer terraformInitMutex.Unlock()
		terraformInitMutex.Lock()
	}

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
	defer func(signalChannel *SignalsForwarder) {
		err := signalChannel.Close()
		if err != nil {
			terragruntOptions.Logger.Warnf("Error closing signal channel: %v", err)
		}
	}(&signalChannel)

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
func isTerraformCommandThatNeedsPty(args []string) (bool, error) {
	if len(args) == 0 || !util.ListContainsElement(terraformCommandsThatNeedPty, args[0]) {
		return false, nil
	}

	fi, err := os.Stdin.Stat()
	if err != nil {
		return false, errors.WithStackTrace(err)
	}

	// if there is data in the stdin, then the terraform console is used in non-interactive mode, for example `echo "1 + 5" | terragrunt console`.
	if fi.Size() > 0 {
		return false, nil
	}

	return true, nil
}

// Return the exit code of a command. If the error does not implement iErrorCode or is not an exec.ExitError
// or *multierror.Error type, the error is returned.
func GetExitCode(err error) (int, error) {
	// Interface to determine if we can retrieve an exit status from an error
	type iErrorCode interface {
		ExitStatus() (int, error)
	}

	if exiterr, ok := errors.Unwrap(err).(iErrorCode); ok {
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
				logger.Debugf("%s signal received. Gracefully shutting down... (it can take up to %v)", cases.Title(language.English).String(s.String()), signalForwardingDelay)

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
	opts, err := options.NewTerragruntOptionsWithConfigPath(path)
	if err != nil {
		return "", err
	}
	opts.Env = terragruntOptions.Env
	opts.Writer = &stdout
	opts.ErrWriter = &stderr
	cmd, err := RunShellCommandWithOutput(opts, path, true, false, "git", "rev-parse", "--show-toplevel")
	terragruntOptions.Logger.Debugf("git show-toplevel result: \n%v\n%v\n", stdout.String(), stderr.String())
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
