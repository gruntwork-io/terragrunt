package main

import (
	"context"
	"fmt"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/plugin"
	"github.com/gruntwork-io/terragrunt/shell"

	"os/exec"
)

type ShellPlugin struct{}

func (p *ShellPlugin) Run(ctx context.Context,
	terragruntOptions *options.TerragruntOptions,
	workingDir string,
	allocatePseudoTty bool,
	cmd *exec.Cmd) (*shell.CmdOutput, error) {

	fmt.Println("Running shell command:", cmd)
	return nil, nil
}

func Plugin() plugin.Plugin {
	return &ShellPlugin{}
}
