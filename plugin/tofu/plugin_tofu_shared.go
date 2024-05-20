package main

import (
	"context"
	"fmt"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/plugin"
	"github.com/gruntwork-io/terragrunt/shell"

	"os/exec"
)

type TofuPlugin struct{}

func (p *TofuPlugin) Run(ctx context.Context,
	terragruntOptions *options.TerragruntOptions,
	workingDir string,
	allocatePseudoTty bool,
	cmd *exec.Cmd) (*shell.CmdOutput, error) {

	fmt.Println("Running tofu command:", cmd)
	return nil, nil
}

func Plugin() plugin.Plugin {
	return &TofuPlugin{}
}
