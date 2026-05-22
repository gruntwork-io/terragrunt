package run

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"

	"github.com/gruntwork-io/terragrunt/internal/cloner"
	"github.com/gruntwork-io/terragrunt/internal/runner/runcfg"
	"github.com/gruntwork-io/terragrunt/internal/shell"
	"github.com/gruntwork-io/terragrunt/internal/telemetry"
	"github.com/gruntwork-io/terragrunt/internal/tflint"
	"github.com/gruntwork-io/terragrunt/internal/util"
	"github.com/gruntwork-io/terragrunt/internal/vexec"
	"github.com/gruntwork-io/terragrunt/pkg/log"
)

const (
	HookCtxTFPathEnvName   = "TG_CTX_TF_PATH"
	HookCtxCommandEnvName  = "TG_CTX_COMMAND"
	HookCtxHookNameEnvName = "TG_CTX_HOOK_NAME"
)

// ProcessHooksParams groups the configuration and data inputs for ProcessHooks.
type ProcessHooksParams struct {
	Opts               *Options
	Cfg                *runcfg.RunConfig
	PreviousExecErrors []error
	Hooks              []runcfg.Hook
}

// ProcessHooks processes a list of hooks, executing each one that matches the current command.
func ProcessHooks(ctx context.Context, l log.Logger, v Venv, p ProcessHooksParams) error {
	if len(p.Hooks) == 0 {
		return nil
	}

	var errorsOccured []error

	l.Debugf("Detected %d Hooks", len(p.Hooks))

	for i := range p.Hooks {
		curHook := &p.Hooks[i]
		if !curHook.If {
			l.Debugf("Skipping hook: %s", curHook.Name)
			continue
		}

		allPreviousErrors := append(slices.Clone(p.PreviousExecErrors), errorsOccured...)
		if shouldRunHook(curHook, p.Opts, allPreviousErrors) {
			err := telemetry.TelemeterFromContext(ctx).Collect(ctx, "hook_"+curHook.Name, map[string]any{
				"hook": curHook.Name,
				"dir":  curHook.WorkingDir,
			}, func(ctx context.Context) error {
				return runHook(ctx, l, v, p.Opts, p.Cfg, curHook)
			})
			if err != nil {
				errorsOccured = append(errorsOccured, err)
			}
		}
	}

	return errors.Join(errorsOccured...)
}

// ProcessErrorHooks runs error_hook blocks whose OnErrors regex matches one
// of previousExecErrors. It is the error-path complement to [ProcessHooks].
func ProcessErrorHooks(
	ctx context.Context,
	l log.Logger,
	exec vexec.Exec,
	hooks []runcfg.ErrorHook,
	opts *Options,
	previousExecErrors []error,
) error {
	if len(hooks) == 0 || len(previousExecErrors) == 0 {
		return nil
	}

	var errorsOccured []error

	l.Debugf("Detected %d error Hooks", len(hooks))

	errorMessages := make([]string, 0, len(previousExecErrors))
	for _, e := range previousExecErrors {
		errorMessage := e.Error()
		// Process execution errors carry stdout that hook patterns need to match against.
		// https://github.com/gruntwork-io/terragrunt/issues/2045
		var processError util.ProcessExecutionError
		if errors.As(e, &processError) {
			errorMessage = fmt.Sprintf("%s\n%s", processError.Error(), processError.Output.Stdout.String())
		}

		errorMessages = append(errorMessages, errorMessage)
	}

	errorMessage := strings.Join(errorMessages, "\n")

	for _, curHook := range hooks {
		if util.MatchesAny(curHook.OnErrors, errorMessage) && slices.Contains(curHook.Commands, opts.TerraformCommand) {
			err := telemetry.TelemeterFromContext(ctx).Collect(ctx, "error_hook_"+curHook.Name, map[string]any{
				"hook": curHook.Name,
				"dir":  curHook.WorkingDir,
			}, func(ctx context.Context) error {
				l.Infof("Executing hook: %s", curHook.Name)

				actionToExecute := curHook.Execute[0]
				actionParams := curHook.Execute[1:]
				hookOpts := optsWithHookEnvs(opts, curHook.Name)

				_, possibleError := shell.RunCommandWithOutput(
					ctx,
					l,
					exec,
					hookOpts.shellRunOptions(),
					curHook.WorkingDir,
					curHook.SuppressStdout,
					false,
					actionToExecute, actionParams...,
				)
				if possibleError != nil {
					l.Errorf("%s", hookErrorMessage(curHook.Name, possibleError))
					return possibleError
				}

				return nil
			})
			if err != nil {
				errorsOccured = append(errorsOccured, err)
			}
		}
	}

	return errors.Join(errorsOccured...)
}

// hookErrorMessage extracts command, args and output from the error
// so users see WHY a hook failed, not just the exit code.
func hookErrorMessage(hookName string, err error) string {
	var processErr util.ProcessExecutionError
	if !errors.As(err, &processErr) {
		return fmt.Sprintf("Hook %q failed to execute: %v", hookName, err)
	}

	exitCode, exitCodeErr := processErr.ExitStatus()
	if exitCodeErr != nil {
		return fmt.Sprintf("Hook %q failed to execute: %v", hookName, err)
	}

	cmd := strings.Join(append([]string{processErr.Command}, processErr.Args...), " ")

	output := strings.TrimSpace(processErr.Output.Stderr.String())
	if output == "" {
		output = strings.TrimSpace(processErr.Output.Stdout.String())
	}

	if output != "" {
		return fmt.Sprintf("Hook %q (command: %s) exited with non-zero exit code %d:\n%s", hookName, cmd, exitCode, output)
	}

	return fmt.Sprintf("Hook %q (command: %s) exited with non-zero exit code %d", hookName, cmd, exitCode)
}

func shouldRunHook(
	hook *runcfg.Hook,
	opts *Options,
	previousExecErrors []error,
) bool {
	// If there's no previous error, execute command.
	// Or if a previous error did happen and the hook opts in via RunOnError, execute.
	// Skip execution when there was an error and the hook doesn't run on errors.
	//
	// resolves: https://github.com/gruntwork-io/terragrunt/issues/459
	hasErrors := len(previousExecErrors) > 0
	isCommandInHook := slices.Contains(hook.Commands, opts.TerraformCommand)

	return isCommandInHook && (!hasErrors || hook.RunOnError)
}

func runHook(
	ctx context.Context,
	l log.Logger,
	v Venv,
	opts *Options,
	cfg *runcfg.RunConfig,
	curHook *runcfg.Hook,
) error {
	l.Infof("Executing hook: %s", curHook.Name)

	workingDir := curHook.WorkingDir
	suppressStdout := curHook.SuppressStdout

	actionToExecute := curHook.Execute[0]
	actionParams := curHook.Execute[1:]
	hookOpts := optsWithHookEnvs(opts, curHook.Name)

	if actionToExecute == "tflint" {
		return executeTFLint(ctx, l, v, opts, cfg, curHook, workingDir)
	}

	_, possibleError := shell.RunCommandWithOutput(
		ctx,
		l,
		v.Exec,
		hookOpts.shellRunOptions(),
		workingDir,
		suppressStdout,
		false,
		actionToExecute, actionParams...,
	)
	if possibleError != nil {
		l.Errorf("%s", hookErrorMessage(curHook.Name, possibleError))
	}

	return possibleError
}

func executeTFLint(
	ctx context.Context,
	l log.Logger,
	v Venv,
	opts *Options,
	cfg *runcfg.RunConfig,
	curHook *runcfg.Hook,
	workingDir string,
) error {
	// fetching source code changes lock since tflint is not thread safe
	rawActualLock, _ := sourceChangeLocks.LoadOrStore(workingDir, &sync.Mutex{})
	actualLock := rawActualLock.(*sync.Mutex)

	actualLock.Lock()
	defer actualLock.Unlock()

	err := tflint.RunTflintWithOpts(ctx, l, v.tflintVenv(), opts.tflintRunOptions(), cfg, curHook)
	if err != nil {
		l.Errorf("%s", hookErrorMessage(curHook.Name, err))
		return err
	}

	return nil
}

func optsWithHookEnvs(opts *Options, hookName string) *Options {
	newOpts := *opts
	newOpts.Env = cloner.Clone(opts.Env)
	newOpts.Env[HookCtxTFPathEnvName] = opts.TFPath
	newOpts.Env[HookCtxCommandEnvName] = opts.TerraformCommand
	newOpts.Env[HookCtxHookNameEnvName] = hookName

	return &newOpts
}
