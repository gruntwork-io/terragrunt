package configstack

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/gruntwork-io/terragrunt/config"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/tf"
	"github.com/gruntwork-io/terragrunt/util"
)

// StackRunner defines an interface for running operations on a Stack.
// It provides methods to execute, represent as string, graph, and log the deploy order of modules within a Stack.
type StackRunner interface {
	Run(ctx context.Context, stack *Stack, l log.Logger, opts *options.TerragruntOptions) error
	String(stack *Stack) string
	Graph(stack *Stack, l log.Logger, opts *options.TerragruntOptions)
	LogModuleDeployOrder(stack *Stack, l log.Logger, terraformCommand string) error
}

type DefaultStackRunner struct{}

func (DefaultStackRunner) String(stack *Stack) string {
	modules := []string{}
	for _, module := range stack.Modules {
		modules = append(modules, "  => "+module.String())
	}
	sort.Strings(modules)
	return fmt.Sprintf("Stack at %s:\n%s", stack.terragruntOptions.WorkingDir, strings.Join(modules, "\n"))
}

func (DefaultStackRunner) LogModuleDeployOrder(stack *Stack, l log.Logger, terraformCommand string) error {
	outStr := fmt.Sprintf("The stack at %s will be processed in the following order for command %s:\n", stack.terragruntOptions.WorkingDir, terraformCommand)

	runGraph, err := stack.GetModuleRunGraph(terraformCommand)
	if err != nil {
		return err
	}

	for i, group := range runGraph {
		outStr += fmt.Sprintf("Group %d\n", i+1)
		for _, module := range group {
			outStr += fmt.Sprintf("- Module %s\n", module.Path)
		}
		outStr += "\n"
	}

	l.Info(outStr)
	return nil
}

func (DefaultStackRunner) Graph(stack *Stack, l log.Logger, opts *options.TerragruntOptions) {
	err := stack.Modules.WriteDot(l, opts.Writer, opts)
	if err != nil {
		l.Warnf("Failed to graph dot: %v", err)
	}
}

func (DefaultStackRunner) Run(ctx context.Context, stack *Stack, l log.Logger, opts *options.TerragruntOptions) error {
	stackCmd := opts.TerraformCommand

	// prepare folder for output hierarchy if output folder is set
	if opts.OutputFolder != "" {
		for _, module := range stack.Modules {
			planFile := module.outputFile(l, opts)
			planDir := filepath.Dir(planFile)
			if err := os.MkdirAll(planDir, os.ModePerm); err != nil {
				return err
			}
		}
	}

	// For any command that needs input, run in non-interactive mode to avoid cominglint stdin across multiple
	// concurrent runs.
	if util.ListContainsElement(config.TerraformCommandsNeedInput, stackCmd) {
		// to support potential positional args in the args list, we append the input=false arg after the first element,
		// which is the target command.
		opts.TerraformCliArgs = util.StringListInsert(opts.TerraformCliArgs, "-input=false", 1)
		stack.syncTerraformCliArgs(l, opts)
	}

	// For apply and destroy, run with auto-approve (unless explicitly disabled) due to the co-mingling of the prompts.
	// This is not ideal, but until we have a better way of handling interactivity with run --all, we take the evil of
	// having a global prompt (managed in cli/cli_app.go) be the gate keeper.
	switch stackCmd {
	case tf.CommandNameApply, tf.CommandNameDestroy:
		if opts.RunAllAutoApprove {
			opts.TerraformCliArgs = util.StringListInsert(opts.TerraformCliArgs, "-auto-approve", 1)
		}
		stack.syncTerraformCliArgs(l, opts)
	case tf.CommandNameShow:
		stack.syncTerraformCliArgs(l, opts)
	case tf.CommandNamePlan:
		// We capture the out stream for each module
		errorStreams := make([]bytes.Buffer, len(stack.Modules))
		for n, module := range stack.Modules {
			module.TerragruntOptions.ErrWriter = io.MultiWriter(&errorStreams[n], module.TerragruntOptions.ErrWriter)
		}
		defer stack.summarizePlanAllErrors(l, errorStreams)
	}

	switch {
	case opts.IgnoreDependencyOrder:
		return stack.Modules.RunModulesIgnoreOrder(ctx, opts, opts.Parallelism)
	case stackCmd == tf.CommandNameDestroy:
		return stack.Modules.RunModulesReverseOrder(ctx, opts, opts.Parallelism)
	default:
		return stack.Modules.RunModules(ctx, opts, opts.Parallelism)
	}
}
