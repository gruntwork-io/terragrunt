package terraform

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/gruntwork-io/terragrunt/telemetry"

	"github.com/gruntwork-io/terragrunt/config"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/shell"
	"github.com/gruntwork-io/terragrunt/tflint"
	"github.com/gruntwork-io/terragrunt/util"
	"github.com/hashicorp/go-multierror"
)

const (
	// HookCtxTFPathEnvName is the name of the environment variable that will be set to
	// give context to the hook about the OpenTofu/Terraform binary being used.
	HookCtxTFPathEnvName = "TG_CTX_TF_PATH"

	// HookCtxCommandEnvName is the name of the environment variable that will be set to
	// give context to the hook about the OpenTofu/Terraform command being executed.
	HookCtxCommandEnvName = "TG_CTX_COMMAND"

	// HookCtxHookNameEnvName is the name of the environment variable that will be set to
	// give context to the hook about the name of the hook being executed.
	HookCtxHookNameEnvName = "TG_CTX_HOOK_NAME"
)

func processErrorHooks(ctx context.Context, hooks []config.ErrorHook, terragruntOptions *options.TerragruntOptions, previousExecErrors *multierror.Error) error { //nolint:lll
	if len(hooks) == 0 || previousExecErrors.ErrorOrNil() == nil {
		return nil
	}

	var errorsOccured *multierror.Error

	terragruntOptions.Logger.Debugf("Detected %d error Hooks", len(hooks))

	customMultierror := multierror.Error{
		Errors: previousExecErrors.Errors,
		ErrorFormat: func(err []error) string {
			result := ""
			for _, e := range err {
				errorMessage := e.Error()
				// Check if is process execution error and try to extract output
				// https://github.com/gruntwork-io/terragrunt/issues/2045
				originalError := errors.Unwrap(e)
				if originalError != nil {
					var processExecutionError util.ProcessExecutionError
					ok := errors.As(originalError, &processExecutionError)
					if ok {
						errorMessage = fmt.Sprintf("%s\n%s", processExecutionError.StdOut, processExecutionError.Stderr)
					}
				}
				result = fmt.Sprintf("%s\n%s", result, errorMessage)
			}

			return result
		},
	}
	errorMessage := customMultierror.Error()

	for _, curHook := range hooks {
		if util.MatchesAny(curHook.OnErrors, errorMessage) && util.ListContainsElement(curHook.Commands, terragruntOptions.TerraformCommand) { //nolint:lll
			terragruntOptions.Logger.Infof("Executing hook: %s", curHook.Name)

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

			_, possibleError := shell.RunShellCommandWithOutput(
				ctx,
				terragruntOptions,
				workingDir,
				suppressStdout,
				false,
				actionToExecute, actionParams...,
			)
			if possibleError != nil {
				terragruntOptions.Logger.Errorf("Error running hook %s with message: %s", curHook.Name, possibleError.Error())
				errorsOccured = multierror.Append(errorsOccured, possibleError)
			}
		}
	}

	return fmt.Errorf("error running error hooks: %w", errorsOccured.ErrorOrNil())
}

func processHooks(ctx context.Context, hooks []config.Hook, terragruntOptions *options.TerragruntOptions, terragruntConfig *config.TerragruntConfig, previousExecErrors *multierror.Error) error { //nolint:lll
	if len(hooks) == 0 {
		return nil
	}

	var errorsOccured *multierror.Error

	terragruntOptions.Logger.Debugf("Detected %d Hooks", len(hooks))

	for _, curHook := range hooks {
		allPreviousErrors := multierror.Append(previousExecErrors, errorsOccured)
		if shouldRunHook(curHook, terragruntOptions, allPreviousErrors) {
			err := telemetry.Telemetry(ctx, terragruntOptions, "hook_"+curHook.Name, map[string]interface{}{
				"hook": curHook.Name,
				"dir":  curHook.WorkingDir,
			}, func(_ context.Context) error { // TODO: Find out why this is being ignored instead of being used
				return runHook(ctx, terragruntOptions, terragruntConfig, curHook)
			})
			if err != nil {
				errorsOccured = multierror.Append(errorsOccured, err)
			}
		}
	}

	return fmt.Errorf("error running hooks: %w", errorsOccured.ErrorOrNil())
}

// shouldRunHook determines if a hook should be run based on the hook configuration and the previous execution errors.
//
// If there are no previous errors, the hook should run. If there are previous errors, the hook should run if the
// RunOnError flag is set to true.
func shouldRunHook(hook config.Hook, terragruntOptions *options.TerragruntOptions, previousExecErrors *multierror.Error) bool { //nolint:lll
	hasErrors := previousExecErrors.ErrorOrNil() != nil
	isCommandInHook := util.ListContainsElement(hook.Commands, terragruntOptions.TerraformCommand)

	return isCommandInHook && (!hasErrors || (hook.RunOnError != nil && *hook.RunOnError))
}

func runHook(ctx context.Context, terragruntOptions *options.TerragruntOptions, terragruntConfig *config.TerragruntConfig, curHook config.Hook) error { //nolint:lll
	terragruntOptions.Logger.Infof("Executing hook: %s", curHook.Name)

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
		if err := executeTFLint(ctx, terragruntOptions, terragruntConfig, curHook, workingDir); err != nil {
			return err
		}
	} else {
		_, possibleError := shell.RunShellCommandWithOutput(
			ctx,
			terragruntOptions,
			workingDir,
			suppressStdout,
			false,
			actionToExecute, actionParams...,
		)
		if possibleError != nil {
			terragruntOptions.Logger.Errorf("Error running hook %s with message: %s", curHook.Name, possibleError.Error())

			return fmt.Errorf("error running hook %s: %w", curHook.Name, possibleError)
		}
	}

	return nil
}

var (
	// ErrFailedToAcquireSourceChangeLock is the error returned when the source change lock could not be acquired.
	ErrFailedToAcquireSourceChangeLock = errors.New("failed to acquire source change lock")
)

func executeTFLint(ctx context.Context, terragruntOptions *options.TerragruntOptions, terragruntConfig *config.TerragruntConfig, curHook config.Hook, workingDir string) error { //nolint:lll
	// fetching source code changes lock since tflint is not thread safe
	rawActualLock, _ := sourceChangeLocks.LoadOrStore(workingDir, &sync.Mutex{})

	actualLock, ok := rawActualLock.(*sync.Mutex)
	if !ok {
		return ErrFailedToAcquireSourceChangeLock
	}

	actualLock.Lock()
	defer actualLock.Unlock()

	err := tflint.RunTflintWithOpts(ctx, terragruntOptions, terragruntConfig, curHook)
	if err != nil {
		terragruntOptions.Logger.Errorf("Error running hook %s with message: %s", curHook.Name, err.Error())

		return fmt.Errorf("error running hook %s: %w", curHook.Name, err)
	}

	return nil
}

func terragruntOptionsWithHookEnvs(opts *options.TerragruntOptions, hookName string) *options.TerragruntOptions {
	newOpts := *opts
	newOpts.Env = util.CloneStringMap(opts.Env)
	newOpts.Env[HookCtxTFPathEnvName] = opts.TerraformPath
	newOpts.Env[HookCtxCommandEnvName] = opts.TerraformCommand
	newOpts.Env[HookCtxHookNameEnvName] = hookName

	return &newOpts
}
