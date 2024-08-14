package shell

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"time"

	"github.com/gruntwork-io/terragrunt/internal/cache"

	"github.com/gruntwork-io/terragrunt/engine"

	"github.com/gruntwork-io/terragrunt/telemetry"

	"github.com/hashicorp/go-version"

	"github.com/gruntwork-io/go-commons/errors"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/util"
	"github.com/sirupsen/logrus"
)

// The signal can be sent to the main process (only `terragrunt`) as well as the process group (`terragrunt` and `terraform`), for example:
// kill -INT <pid>  # sends SIGINT only to the main process
// kill -INT -<pid> # sends SIGINT to the process group
// Since we cannot know how the signal is sent, we should give `terraform` time to gracefully exit if it receives the signal directly from the shell, to avoid sending the second interrupt signal to `terraform`.
const SignalForwardingDelay = time.Second * 30

const (
	gitPrefix = "git::"
	refsTags  = "refs/tags/"

	tagSplitPart = 2
)

// Commands that implement a REPL need a pseudo TTY when run as a subprocess in order for the readline properties to be
// preserved. This is a list of terraform commands that have this property, which is used to determine if terragrunt
// should allocate a ptty when running that terraform command.
var terraformCommandsThatNeedPty = []string{
	"console",
}

// Run the given Terraform command
func RunTerraformCommand(ctx context.Context, terragruntOptions *options.TerragruntOptions, args ...string) error {
	needPTY, err := isTerraformCommandThatNeedsPty(args)
	if err != nil {
		return err
	}

	_, err = RunShellCommandWithOutput(ctx, terragruntOptions, "", false, needPTY, terragruntOptions.TerraformPath, args...)
	return err
}

// Run the given shell command
func RunShellCommand(ctx context.Context, terragruntOptions *options.TerragruntOptions, command string, args ...string) error {
	_, err := RunShellCommandWithOutput(ctx, terragruntOptions, "", false, false, command, args...)
	return err
}

// Run the given Terraform command, writing its stdout/stderr to the terminal AND returning stdout/stderr to this
// method's caller
func RunTerraformCommandWithOutput(ctx context.Context, terragruntOptions *options.TerragruntOptions, args ...string) (*util.CmdOutput, error) {
	needPTY, err := isTerraformCommandThatNeedsPty(args)
	if err != nil {
		return nil, err
	}

	return RunShellCommandWithOutput(ctx, terragruntOptions, "", false, needPTY, terragruntOptions.TerraformPath, args...)
}

// Run the specified shell command with the specified arguments. Connect the command's stdin, stdout, and stderr to
// the currently running app. The command can be executed in a custom working directory by using the parameter
// `workingDir`. Terragrunt working directory will be assumed if empty string.
func RunShellCommandWithOutput(
	ctx context.Context,
	terragruntOptions *options.TerragruntOptions,
	workingDir string,
	suppressStdout bool,
	allocatePseudoTty bool,
	command string,
	args ...string,
) (*util.CmdOutput, error) {
	if command == terragruntOptions.TerraformPath {
		if fn := TerraformCommandHookFromContext(ctx); fn != nil {
			return fn(ctx, terragruntOptions, args)
		}
	}

	var output *util.CmdOutput = nil
	var commandDir = workingDir
	if workingDir == "" {
		commandDir = terragruntOptions.WorkingDir
	}
	err := telemetry.Telemetry(ctx, terragruntOptions, "run_"+command, map[string]interface{}{
		"command": command,
		"args":    fmt.Sprintf("%v", args),
		"dir":     commandDir,
	}, func(childCtx context.Context) error {
		terragruntOptions.Logger.Debugf("Running command: %s %s", command, strings.Join(args, " "))
		if suppressStdout {
			terragruntOptions.Logger.Debugf("Command output will be suppressed.")
		}

		var stdoutBuf bytes.Buffer
		var stderrBuf bytes.Buffer

		cmd := exec.Command(command, args...)

		// TODO: consider adding prefix from terragruntOptions logger to stdout and stderr
		cmd.Env = toEnvVarsList(terragruntOptions.Env)

		var outWriter = terragruntOptions.Writer
		var errWriter = terragruntOptions.ErrWriter

		// redirect output through logger with json wrapping
		if terragruntOptions.JsonLogFormat && terragruntOptions.TerraformLogsToJson {

			jsonWriter := terragruntOptions.Logger.Logger.WithField("workingDir", terragruntOptions.WorkingDir).WithField("executedCommandArgs", args)
			jsonWriter.Logger.Out = outWriter
			outWriter = jsonWriter.Writer()

			jsonErrorWriter := terragruntOptions.Logger.Logger.WithField("workingDir", terragruntOptions.WorkingDir).WithField("executedCommandArgs", args)
			jsonErrorWriter.Logger.Out = errWriter
			errWriter = jsonErrorWriter.WriterLevel(logrus.ErrorLevel)
		}

		var prefix = ""
		if terragruntOptions.IncludeModulePrefix {
			prefix = terragruntOptions.OutputPrefix
		}
		cmd.Dir = commandDir

		// Inspired by https://blog.kowalczyk.info/article/wOYk/advanced-command-execution-in-go-with-osexec.html
		cmdStderr := io.MultiWriter(withPrefix(errWriter, prefix), &stderrBuf)
		var cmdStdout io.Writer
		if !suppressStdout {
			cmdStdout = io.MultiWriter(withPrefix(outWriter, prefix), &stdoutBuf)
		} else {
			cmdStdout = io.MultiWriter(&stdoutBuf)
		}

		if command == terragruntOptions.TerraformPath && terragruntOptions.Engine != nil && !engine.IsEngineEnabled() {
			terragruntOptions.Logger.Debugf("Engine is not enabled, running command directly in %s", commandDir)
		}
		useEngine := terragruntOptions.Engine != nil && engine.IsEngineEnabled()
		// If the engine is enabled and the command is IaC executable, use the engine to run the command.
		if useEngine && command == terragruntOptions.TerraformPath {
			terragruntOptions.Logger.Debugf("Using engine to run command: %s %s", command, strings.Join(args, " "))
			cmdOutput, err := engine.Run(ctx, &engine.ExecutionOptions{
				TerragruntOptions: terragruntOptions,
				CmdStdout:         cmdStdout,
				CmdStderr:         cmdStderr,
				WorkingDir:        cmd.Dir,
				SuppressStdout:    suppressStdout,
				AllocatePseudoTty: allocatePseudoTty,
				Command:           command,
				Args:              args,
			})
			if err != nil {
				return errors.WithStackTrace(err)
			}
			output = cmdOutput
			return err
		}

		// If we need to allocate a ptty for the command, route through the ptty routine. Otherwise, directly call the
		// command.
		if allocatePseudoTty {
			if err := runCommandWithPTTY(terragruntOptions, cmd, cmdStdout, cmdStderr); err != nil {
				return err
			}
		} else {
			cmd.Stdin = os.Stdin
			cmd.Stdout = cmdStdout
			cmd.Stderr = cmdStderr
			if err := cmd.Start(); err != nil {
				// bad path, binary not executable, &c
				return errors.WithStackTrace(err)
			}
		}

		// Make sure to forward signals to the subcommand.
		cmdChannel := make(chan error) // used for closing the signals forwarder goroutine
		signalChannel := NewSignalsForwarder(InterruptSignals, cmd, terragruntOptions.Logger, cmdChannel)
		defer func(signalChannel *SignalsForwarder) {
			err := signalChannel.Close()
			if err != nil {
				terragruntOptions.Logger.Warnf("Error closing signal channel: %v", err)
			}
		}(&signalChannel)

		err := cmd.Wait()
		cmdChannel <- err

		cmdOutput := util.CmdOutput{
			Stdout: stdoutBuf.String(),
			Stderr: stderrBuf.String(),
		}

		if err != nil {
			err = util.ProcessExecutionError{
				Err:        err,
				StdOut:     stdoutBuf.String(),
				Stderr:     stderrBuf.String(),
				WorkingDir: cmd.Dir,
			}
		}
		output = &cmdOutput
		return errors.WithStackTrace(err)
	})
	return output, err
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

func withPrefix(writer io.Writer, prefix string) io.Writer {
	if prefix == "" {
		return writer
	}

	return util.PrefixedWriter(writer, prefix)
}

type SignalsForwarder chan os.Signal

// NewSignalsForwarder Forwards signals to a command, waiting for the command to finish.
func NewSignalsForwarder(signals []os.Signal, c *exec.Cmd, logger *logrus.Entry, cmdChannel chan error) SignalsForwarder {
	signalChannel := make(chan os.Signal, 1)
	signal.Notify(signalChannel, signals...)

	go func() {
		for {
			select {
			case s := <-signalChannel:
				select {
				case <-time.After(SignalForwardingDelay):
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

// GitTopLevelDir - fetch git repository path from passed directory
func GitTopLevelDir(ctx context.Context, terragruntOptions *options.TerragruntOptions, path string) (string, error) {
	runCache := cache.ContextCache[string](ctx, RunCmdCacheContextKey)
	cacheKey := "top-level-dir-" + path
	if gitTopLevelDir, found := runCache.Get(ctx, cacheKey); found {
		return gitTopLevelDir, nil
	}
	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}
	opts, err := options.NewTerragruntOptionsWithConfigPath(path)
	if err != nil {
		return "", err
	}
	opts.Env = terragruntOptions.Env
	opts.Writer = &stdout
	opts.ErrWriter = &stderr
	cmd, err := RunShellCommandWithOutput(ctx, opts, path, true, false, "git", "rev-parse", "--show-toplevel")
	if err != nil {
		return "", err
	}
	cmdOutput := strings.TrimSpace(cmd.Stdout)
	terragruntOptions.Logger.Debugf("git show-toplevel result: \n%v\n%v\n%v\n", stdout.String(), stderr.String(), cmdOutput)
	runCache.Put(ctx, cacheKey, cmdOutput)
	return cmdOutput, nil
}

// GitRepoTags - fetch git repository tags from passed url
func GitRepoTags(ctx context.Context, opts *options.TerragruntOptions, gitRepo *url.URL) ([]string, error) {
	repoPath := gitRepo.String()
	// remove git:: part if present
	repoPath = strings.TrimPrefix(repoPath, gitPrefix)

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}
	gitOpts, err := options.NewTerragruntOptionsWithConfigPath(opts.WorkingDir)
	if err != nil {
		return nil, err
	}
	gitOpts.Env = opts.Env
	gitOpts.Writer = &stdout
	gitOpts.ErrWriter = &stderr

	output, err := RunShellCommandWithOutput(ctx, opts, opts.WorkingDir, true, false, "git", "ls-remote", "--tags", repoPath)
	if err != nil {
		return nil, errors.WithStackTrace(err)
	}
	var tags []string
	tagLines := strings.Split(output.Stdout, "\n")
	for _, line := range tagLines {
		fields := strings.Fields(line)
		if len(fields) >= tagSplitPart {
			tags = append(tags, fields[1])
		}
	}
	return tags, nil
}

// GitLastReleaseTag - fetch git repository last release tag
func GitLastReleaseTag(ctx context.Context, opts *options.TerragruntOptions, gitRepo *url.URL) (string, error) {
	tags, err := GitRepoTags(ctx, opts, gitRepo)
	if err != nil {
		return "", err
	}
	if len(tags) == 0 {
		return "", nil
	}
	return LastReleaseTag(tags), nil
}

// LastReleaseTag - return last release tag from passed tags slice.
func LastReleaseTag(tags []string) string {
	semverTags := extractSemVerTags(tags)
	if len(semverTags) == 0 {
		return ""
	}
	// find last semver tag
	lastVersion := semverTags[0]
	for _, ver := range semverTags {
		if ver.GreaterThanOrEqual(lastVersion) {
			lastVersion = ver
		}
	}
	return lastVersion.Original()
}

// extractSemVerTags - extract semver tags from passed tags slice.
func extractSemVerTags(tags []string) []*version.Version {
	var semverTags []*version.Version
	for _, tag := range tags {
		t := strings.TrimPrefix(tag, refsTags)
		if v, err := version.NewVersion(t); err == nil {
			// consider only semver tags
			semverTags = append(semverTags, v)
		}
	}
	return semverTags
}
