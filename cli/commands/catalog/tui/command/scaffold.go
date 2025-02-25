// Package command provides the implementation of the terragrunt scaffold command
// This command is used to scaffold a new Terragrunt unit in the current directory.
package command

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/gruntwork-io/terragrunt/cli/commands/catalog/module"

	"github.com/gruntwork-io/terragrunt/cli/commands/scaffold"
	"github.com/gruntwork-io/terragrunt/options"
)

type Scaffold struct {
	module            *module.Module
	terragruntOptions *options.TerragruntOptions
}

func NewScaffold(opts *options.TerragruntOptions, module *module.Module) *Scaffold {
	return &Scaffold{
		module:            module,
		terragruntOptions: opts,
	}
}

func (cmd *Scaffold) Run() error {
	sourcePath := cmd.module.TerraformSourcePath()
	cmd.module.Logger().Infof("Run Scaffold for the module: %q", sourcePath)

	// For cln:// URLs, we need to strip the protocol for the scaffold command
	if strings.HasPrefix(sourcePath, "cln://") {
		sourcePath = strings.TrimPrefix(sourcePath, "cln://")

		// If it's already in SSH format (git@github.com:org/repo), use it directly
		if strings.HasPrefix(sourcePath, "git@") {
			return scaffold.Run(context.Background(), cmd.terragruntOptions, "git::"+sourcePath, "")
		}

		// Otherwise, convert to SSH format
		sourcePath = strings.TrimPrefix(sourcePath, "github.com/")
		sourcePath = fmt.Sprintf("git@github.com:%s", sourcePath)
		return scaffold.Run(context.Background(), cmd.terragruntOptions, "git::"+sourcePath, "")
	}

	return scaffold.Run(context.Background(), cmd.terragruntOptions, sourcePath, "")
}

func (cmd *Scaffold) SetStdin(io.Reader) {
}

func (cmd *Scaffold) SetStdout(io.Writer) {
}

func (cmd *Scaffold) SetStderr(io.Writer) {
}
