package run

import (
	"context"
	"fmt"
	"sync"

	"github.com/gruntwork-io/terragrunt/config"
	"github.com/gruntwork-io/terragrunt/internal/cloner"
	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/internal/report"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/shell"
	"github.com/gruntwork-io/terragrunt/telemetry"
	"github.com/gruntwork-io/terragrunt/tflint"
	"github.com/gruntwork-io/terragrunt/util"
	"github.com/hashicorp/go-multierror"
)

const (
	HookCtxTFPathEnvName   = "TG_CTX_TF_PATH"
	HookCtxCommandEnvName  = "TG_CTX_COMMAND"
	HookCtxHookNameEnvName = "TG_CTX_HOOK_NAME"
)

func processErrorHooks(ctx context.Context, l log.Logger, hooks []config.ErrorHook, terragruntOptions *options.TerragruntOptions, previousExecErrors *errors.MultiError, _ *report.Report) error {
	if len(hooks) == 0 || previousExecErrors.ErrorOrNil() == nil {
		return nil
	}

	var errorsOccured *multierror.Error

	l.Debugf("Detected %d error Hooks", len(hooks))

	customMultierror := multierror.Error{
		Errors: previousExecErrors.WrappedErrors(),
		ErrorFormat: func(err []error) string {
			result := ""
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
				result = fmt.Sprintf("%s\n%s", result, errorMessage)
			}
			return result
		},
	}
	errorMessage := customMultierror.Error()

	for _, curHook := range hooks {
		if util.MatchesAny(curHook.OnErrors, errorMessage) && util.ListContainsElement(curHook.Commands, terragruntOptions.TerraformCommand) {
			err := telemetry.TelemeterFromContext(ctx).Collect(ctx, "error_hook_"+curHook.Name, map[string]any{
				"hook": curHook.Name,
				"dir":  curHook.WorkingDir,
			}, func(ctx context.Context) error {
				l.Infof("Executing hook: %s", curHook.Name)

				workingDir := ""
				if curHook.WorkingDir != nil {
					workingDir = *curHook.WorkingDir
				}

				var suppressStdout bool
				if curHook.SuppressStdout != nil && *curHook.SuppressStdout {
					suppressStdout = true
				}

				actionToExecute := curHook.Execute[0]
				actionParams := curHook.Execute[1:]
				terragruntOptions = terragruntOptionsWithHookEnvs(terragruntOptions, curHook.Name)

				_, possibleError := shell.RunCommandWithOutput(
					ctx,
					l,
					terragruntOptions,
					workingDir,
					suppressStdout,
					false,
					actionToExecute, actionParams...,
				)
				if possibleError != nil {
					l.Errorf("Error running hook %s with message: %s", curHook.Name, possibleError.Error())
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

func processHooks(
	ctx context.Context,
	l log.Logger,
	hooks []config.Hook,
	opts *options.TerragruntOptions,
	cfg *config.TerragruntConfig,
	previousExecErrors *errors.MultiError,
	_ *report.Report,
) error {
	if len(hooks) == 0 {
		return nil
	}

	var errorsOccured *multierror.Error

	l.Debugf("Detected %d Hooks", len(hooks))

	for _, curHook := range hooks {
		if curHook.If != nil && !*curHook.If {
			l.Debugf("Skipping hook: %s", curHook.Name)
			continue
		}

		allPreviousErrors := previousExecErrors.Append(errorsOccured)
		if shouldRunHook(curHook, opts, allPreviousErrors) {
			err := telemetry.TelemeterFromContext(ctx).Collect(ctx, "hook_"+curHook.Name, map[string]any{
				"hook": curHook.Name,
				"dir":  curHook.WorkingDir,
			}, func(ctx context.Context) error {
				return runHook(ctx, l, opts, cfg, curHook)
			})
			if err != nil {
				errorsOccured = multierror.Append(errorsOccured, err)
			}
		}
	}

	return errorsOccured.ErrorOrNil()
}

func shouldRunHook(hook config.Hook, terragruntOptions *options.TerragruntOptions, previousExecErrors *errors.MultiError) bool {
	// if there's no previous error, execute command
	// OR if a previous error DID happen AND we want to run anyways
	// then execute.
	// Skip execution if there was an error AND we care about errors
	//
	// resolves: https://github.com/gruntwork-io/terragrunt/issues/459
	hasErrors := previousExecErrors.ErrorOrNil() != nil
	isCommandInHook := util.ListContainsElement(hook.Commands, terragruntOptions.TerraformCommand)

	return isCommandInHook && (!hasErrors || (hook.RunOnError != nil && *hook.RunOnError))
}

func runHook(ctx context.Context, l log.Logger, terragruntOptions *options.TerragruntOptions, terragruntConfig *config.TerragruntConfig, curHook config.Hook) error {
	l.Infof("Executing hook: %s", curHook.Name)

	workingDir := ""
	if curHook.WorkingDir != nil {
		workingDir = *curHook.WorkingDir
	}

	var suppressStdout bool
	if curHook.SuppressStdout != nil && *curHook.SuppressStdout {
		suppressStdout = true
	}

	actionToExecute := curHook.Execute[0]
	actionParams := curHook.Execute[1:]
	terragruntOptions = terragruntOptionsWithHookEnvs(terragruntOptions, curHook.Name)

	if actionToExecute == "tflint" {
		if err := executeTFLint(ctx, l, terragruntOptions, terragruntConfig, curHook, workingDir); err != nil {
			return err
		}
	} else {
		_, possibleError := shell.RunCommandWithOutput(
			ctx,
			l,
			terragruntOptions,
			workingDir,
			suppressStdout,
			false,
			actionToExecute, actionParams...,
		)
		if possibleError != nil {
			l.Errorf("Error running hook %s with message: %s", curHook.Name, possibleError.Error())
			return possibleError
		}
	}

	return nil
}

func executeTFLint(ctx context.Context, l log.Logger, opts *options.TerragruntOptions, cfg *config.TerragruntConfig, curHook config.Hook, workingDir string) error {
	// fetching source code changes lock since tflint is not thread safe
	rawActualLock, _ := sourceChangeLocks.LoadOrStore(workingDir, &sync.Mutex{})
	actualLock := rawActualLock.(*sync.Mutex)

	actualLock.Lock()
	defer actualLock.Unlock()

	err := tflint.RunTflintWithOpts(ctx, l, opts, cfg, curHook)
	if err != nil {
		l.Errorf("Error running hook %s with message: %s", curHook.Name, err.Error())
		return err
	}

	return nil
}

func terragruntOptionsWithHookEnvs(opts *options.TerragruntOptions, hookName string) *options.TerragruntOptions {
	newOpts := *opts
	newOpts.Env = cloner.Clone(opts.Env)
	newOpts.Env[HookCtxTFPathEnvName] = opts.TFPath
	newOpts.Env[HookCtxCommandEnvName] = opts.TerraformCommand
	newOpts.Env[HookCtxHookNameEnvName] = hookName

	return &newOpts
}
