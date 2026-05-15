package run

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"sync"

	"github.com/gruntwork-io/terragrunt/internal/cloner"
	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/internal/report"
	"github.com/gruntwork-io/terragrunt/internal/runner/runcfg"
	"github.com/gruntwork-io/terragrunt/internal/shell"
	"github.com/gruntwork-io/terragrunt/internal/telemetry"
	"github.com/gruntwork-io/terragrunt/internal/tflint"
	"github.com/gruntwork-io/terragrunt/internal/util"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/hashicorp/go-multierror"
)

const (
	HookCtxTFPathEnvName   = "TG_CTX_TF_PATH"
	HookCtxCommandEnvName  = "TG_CTX_COMMAND"
	HookCtxHookNameEnvName = "TG_CTX_HOOK_NAME"
)

// ProcessHooks processes a list of hooks, executing each one that matches the current command.
func ProcessHooks(
	ctx context.Context,
	l log.Logger,
	v *Venv,
	hooks []runcfg.Hook,
	opts *Options,
	cfg *runcfg.RunConfig,
	previousExecErrors *errors.MultiError,
	_ *report.Report,
) error {
	if len(hooks) == 0 {
		return nil
	}

	var errorsOccured *multierror.Error

	l.Debugf("Detected %d Hooks", len(hooks))

	for i := range hooks {
		curHook := &hooks[i]
		if !curHook.If {
			l.Debugf("Skipping hook: %s", curHook.Name)
			continue
		}

		allPreviousErrors := previousExecErrors.Append(errorsOccured)
		if shouldRunHook(curHook, opts, allPreviousErrors) {
			err := telemetry.TelemeterFromContext(ctx).Collect(ctx, "hook_"+curHook.Name, map[string]any{
				"hook": curHook.Name,
				"dir":  curHook.WorkingDir,
			}, func(ctx context.Context) error {
				return runHook(ctx, l, v, opts, cfg, curHook)
			})
			if err != nil {
				errorsOccured = multierror.Append(errorsOccured, err)
			}
		}
	}

	return errorsOccured.ErrorOrNil()
}

// ProcessErrorHooks runs error_hook blocks whose OnErrors regex matches one
// of previousExecErrors. It is the error-path complement to [ProcessHooks].
func ProcessErrorHooks(
	ctx context.Context,
	l log.Logger,
	v *Venv,
	hooks []runcfg.ErrorHook,
	opts *Options,
	previousExecErrors *errors.MultiError,
) error {
	if len(hooks) == 0 || previousExecErrors.ErrorOrNil() == nil {
		return nil
	}

	var errorsOccured *multierror.Error

	l.Debugf("Detected %d error Hooks", len(hooks))

	customMultierror := multierror.Error{
		Errors: previousExecErrors.WrappedErrors(),
		ErrorFormat: func(err []error) string {
			errorMessages := make([]string, 0, len(err))

			for _, e := range err {
				errorMessage := e.Error()
				// Check if is process execution error and try to extract output
				// https://github.com/gruntwork-io/terragrunt/issues/2045
				originalError := errors.Unwrap(e)
				if originalError != nil {
					var processError util.ProcessExecutionError
					if ok := errors.As(originalError, &processError); ok {
						errorMessage = fmt.Sprintf("%s\n%s", processError.Error(), processError.Output.Stdout.String())
					}
				}

				errorMessages = append(errorMessages, errorMessage)
			}

			return strings.Join(errorMessages, "\n")
		},
	}
	errorMessage := customMultierror.Error()

	for _, curHook := range hooks {
		if util.MatchesAny(curHook.OnErrors, errorMessage) && slices.Contains(curHook.Commands, opts.TerraformCommand) {
			err := telemetry.TelemeterFromContext(ctx).Collect(ctx, "error_hook_"+curHook.Name, map[string]any{
				"hook": curHook.Name,
				"dir":  curHook.WorkingDir,
			}, func(ctx context.Context) error {
				l.Infof("Executing hook: %s", curHook.Name)

				actionToExecute := curHook.Execute[0]
				actionParams := curHook.Execute[1:]

				hookV := v.ToRoot()
				hookV.Env = hookEnv(v.Env, opts, curHook.Name)

				_, possibleError := shell.RunCommandWithOutput(
					ctx,
					l,
					hookV,
					opts.shellRunOptions(hookV.Env),
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
				errorsOccured = multierror.Append(errorsOccured, err)
			}
		}
	}

	return errorsOccured.ErrorOrNil()
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
	previousExecErrors *errors.MultiError,
) bool {
	// if there's no previous error, execute command
	// OR if a previous error DID happen AND we want to run anyways
	// then execute.
	// Skip execution if there was an error AND we care about errors
	//
	// resolves: https://github.com/gruntwork-io/terragrunt/issues/459
	hasErrors := previousExecErrors.ErrorOrNil() != nil
	isCommandInHook := slices.Contains(hook.Commands, opts.TerraformCommand)

	return isCommandInHook && (!hasErrors || hook.RunOnError)
}

func runHook(
	ctx context.Context,
	l log.Logger,
	v *Venv,
	opts *Options,
	cfg *runcfg.RunConfig,
	curHook *runcfg.Hook,
) error {
	l.Infof("Executing hook: %s", curHook.Name)

	workingDir := curHook.WorkingDir
	suppressStdout := curHook.SuppressStdout

	actionToExecute := curHook.Execute[0]
	actionParams := curHook.Execute[1:]

	if actionToExecute == "tflint" {
		return executeTFLint(ctx, l, v, opts, cfg, curHook, workingDir)
	}

	hookV := v.ToRoot()
	hookV.Env = hookEnv(v.Env, opts, curHook.Name)

	_, possibleError := shell.RunCommandWithOutput(
		ctx,
		l,
		hookV,
		opts.shellRunOptions(hookV.Env),
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
	v *Venv,
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

	err := tflint.RunTflintWithOpts(ctx, l, v.tflintVenv(), opts.tflintRunOptions(v.Env), cfg, curHook)
	if err != nil {
		l.Errorf("%s", hookErrorMessage(curHook.Name, err))
		return err
	}

	return nil
}

// hookEnv clones env and adds the hook context variables (TFPath,
// TerraformCommand, hookName). The returned map is independent of env so
// mutations during hook execution don't leak back to the run-wide
// environment.
func hookEnv(env map[string]string, opts *Options, hookName string) map[string]string {
	cloned := cloner.Clone(env)
	cloned[HookCtxTFPathEnvName] = opts.TFPath
	cloned[HookCtxCommandEnvName] = opts.TerraformCommand
	cloned[HookCtxHookNameEnvName] = hookName

	return cloned
}
