package run

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"sync"

	"github.com/gruntwork-io/terragrunt/internal/cloner"
	"github.com/gruntwork-io/terragrunt/internal/experiment"
	"github.com/gruntwork-io/terragrunt/internal/multierror"
	"github.com/gruntwork-io/terragrunt/internal/runner/runcfg"
	"github.com/gruntwork-io/terragrunt/internal/shell"
	"github.com/gruntwork-io/terragrunt/internal/telemetry"
	"github.com/gruntwork-io/terragrunt/internal/tflint"
	"github.com/gruntwork-io/terragrunt/internal/util"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/pkg/log"
)

const (
	HookCtxTFPathEnvName   = "TG_CTX_TF_PATH"
	HookCtxCommandEnvName  = "TG_CTX_COMMAND"
	HookCtxHookNameEnvName = "TG_CTX_HOOK_NAME"

	// The following are gated behind the hook-context-env experiment.
	HookCtxHookTypeEnvName      = "TG_CTX_HOOK_TYPE"
	HookCtxSourceEnvName        = "TG_CTX_SOURCE"
	HookCtxTerragruntDirEnvName = "TG_CTX_TERRAGRUNT_DIR"
)

const (
	HookTypeUnknown HookType = iota
	HookTypeBefore
	HookTypeAfter
	HookTypeError
)

var hookTypeNames = map[HookType]string{
	HookTypeBefore: "before_hook",
	HookTypeAfter:  "after_hook",
	HookTypeError:  "error_hook",
}

type HookType byte

func hookTypeName(hookType HookType) (string, bool) {
	if name, ok := hookTypeNames[hookType]; ok {
		return name, true
	}

	return "", false
}

// ProcessHooksParams groups the configuration and data inputs for ProcessHooks.
type ProcessHooksParams struct {
	Opts               *Options
	Cfg                *runcfg.RunConfig
	PreviousExecErrors []error
	Hooks              []runcfg.Hook
	HookType           HookType
}

// ProcessHooks processes a list of hooks, executing each one that matches the current command.
func ProcessHooks(ctx context.Context, l log.Logger, v venv.Venv, p ProcessHooksParams) error {
	if len(p.Hooks) == 0 {
		return nil
	}

	if p.Opts != nil && p.Opts.NoHooks {
		l.Debugf("Skipping hooks because --no-hooks is set.")
		return nil
	}

	l.Debugf("Detected %d Hooks", len(p.Hooks))

	// Seed with the errors from earlier stages and append as hooks fail, so each
	// hook's run condition sees the failures before it. The errors from this call
	// are everything past priorCount.
	allPreviousErrors := slices.Clone(p.PreviousExecErrors)
	priorCount := len(allPreviousErrors)

	for i := range p.Hooks {
		curHook := &p.Hooks[i]
		if !curHook.If {
			l.Debugf("Skipping hook: %s", curHook.Name)
			continue
		}

		if shouldRunHook(curHook, p.Opts, allPreviousErrors) {
			err := telemetry.TelemeterFromContext(ctx).Collect(ctx, l, "hook_"+curHook.Name, map[string]any{
				"hook": curHook.Name,
				"dir":  curHook.WorkingDir,
			}, func(ctx context.Context, l log.Logger) error {
				return runHook(ctx, l, v, p.Opts, p.Cfg, curHook, p.HookType)
			})
			if err != nil {
				allPreviousErrors = append(allPreviousErrors, err)
			}
		}
	}

	return multierror.Join(allPreviousErrors[priorCount:]...)
}

// ProcessErrorHooks runs error_hook blocks whose OnErrors regex matches one
// of previousExecErrors. It is the error-path complement to [ProcessHooks].
func ProcessErrorHooks(
	ctx context.Context,
	l log.Logger,
	v venv.Venv,
	hooks []runcfg.ErrorHook,
	cfg *runcfg.RunConfig,
	opts *Options,
	previousExecErrors []error,
) error {
	if len(hooks) == 0 || len(previousExecErrors) == 0 {
		return nil
	}

	if opts != nil && opts.NoHooks {
		l.Debugf("Skipping error hooks because --no-hooks is set.")
		return nil
	}

	var errorsOccured []error

	l.Debugf("Detected %d error Hooks", len(hooks))

	errorMessages := make([]string, 0, len(previousExecErrors))
	for _, e := range previousExecErrors {
		errorMessage := e.Error()
		// Process execution errors carry stdout that hook patterns need to match against.
		// https://github.com/gruntwork-io/terragrunt/issues/2045
		if processError, ok := errors.AsType[util.ProcessExecutionError](e); ok {
			errorMessage = fmt.Sprintf("%s\n%s", processError.Error(), processError.Output.Stdout.String())
		}

		errorMessages = append(errorMessages, errorMessage)
	}

	errorMessage := strings.Join(errorMessages, "\n")

	for _, curHook := range hooks {
		if util.MatchesAny(curHook.OnErrors, errorMessage) && slices.Contains(curHook.Commands, opts.TerraformCommand) {
			err := telemetry.TelemeterFromContext(ctx).Collect(ctx, l, "error_hook_"+curHook.Name, map[string]any{
				"hook": curHook.Name,
				"dir":  curHook.WorkingDir,
			}, func(ctx context.Context, l log.Logger) error {
				l.Infof("Executing hook: %s", curHook.Name)

				actionToExecute := curHook.Execute[0]
				actionParams := curHook.Execute[1:]

				env, hookEnvErr := hookEnv(v.Env, opts, cfg, curHook.Name, HookTypeError)
				if hookEnvErr != nil {
					return hookEnvErr
				}

				hookV := v.WithEnv(env)

				_, possibleError := shell.RunCommandWithOutput(
					ctx,
					l,
					hookV,
					opts.shellRunOptions(),
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

	return multierror.Join(errorsOccured...)
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
	v venv.Venv,
	opts *Options,
	cfg *runcfg.RunConfig,
	curHook *runcfg.Hook,
	hookType HookType,
) error {
	l.Infof("Executing hook: %s", curHook.Name)

	workingDir := curHook.WorkingDir
	suppressStdout := curHook.SuppressStdout

	actionToExecute := curHook.Execute[0]
	actionParams := curHook.Execute[1:]

	if actionToExecute == "tflint" {
		return executeTFLint(ctx, l, v, opts, cfg, curHook, workingDir)
	}

	env, err := hookEnv(v.Env, opts, cfg, curHook.Name, hookType)
	if err != nil {
		return err
	}

	hookV := v.WithEnv(env)

	_, possibleError := shell.RunCommandWithOutput(
		ctx,
		l,
		hookV,
		opts.shellRunOptions(),
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
	v venv.Venv,
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

	err := tflint.RunTflintWithOpts(ctx, l, v, opts.tflintRunOptions(), cfg, curHook)
	if err != nil {
		l.Errorf("%s", hookErrorMessage(curHook.Name, err))
		return err
	}

	return nil
}

// hookEnv clones env and adds the hook context variables (TFPath,
// TerraformCommand, hookName). The returned map is independent of env so
// mutations during hook execution don't leak back to the run-wide
// environment. When the hook-context-env experiment is enabled it also injects
// the TG_CTX_* variables for the hook type, source, and Terragrunt directory.
func hookEnv(env map[string]string, opts *Options, cfg *runcfg.RunConfig, hookName string, hookType HookType) (map[string]string, error) {
	cloned := cloner.Clone(env)
	cloned[HookCtxTFPathEnvName] = opts.TFPath
	cloned[HookCtxCommandEnvName] = opts.TerraformCommand
	cloned[HookCtxHookNameEnvName] = hookName

	if opts.Experiments.Evaluate(experiment.HookContextEnv) {
		hookTypeValue, ok := hookTypeName(hookType)
		if !ok {
			return nil, fmt.Errorf("unknown hook type: %d", hookType)
		}

		source, err := runcfg.GetTerraformSourceURL(opts.Source, opts.SourceMap, opts.OriginalTerragruntConfigPath, cfg)
		if err != nil {
			return nil, err
		}

		cloned[HookCtxHookTypeEnvName] = hookTypeValue
		cloned[HookCtxSourceEnvName] = source
		cloned[HookCtxTerragruntDirEnvName] = filepath.Dir(opts.TerragruntConfigPath)
	}

	return cloned, nil
}
