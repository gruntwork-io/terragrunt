package hclvalidate

import (
	"bufio"
	"context"
	"fmt"
	"os"

	"github.com/gruntwork-io/terragrunt/config"
	"github.com/gruntwork-io/terragrunt/config/hclparse"
	"github.com/gruntwork-io/terragrunt/configstack"
	"github.com/gruntwork-io/terragrunt/options"
)

func Run(ctx context.Context, opts *Options) error {

	parseOption := hclparse.WithSkipErrors()

	opts.RunTerragrunt = func(ctx context.Context, opts *options.TerragruntOptions) error {
		_, err := config.ReadTerragruntConfig(ctx, opts, parseOption)
		if err != nil {
			return err
		}

		jsonOutput := bufio.NewWriter(opts.Writer)
		jsonOutput.Write([]byte("{}"))

		fmt.Println("----------------------", opts.WorkingDir, opts.TerraformCommand)
		return nil
	}

	stack, err := configstack.FindStackInSubfolders(ctx, opts.TerragruntOptions)
	if err != nil {
		return err
	}

	os.Exit(1)

	opts.Logger.Debugf("%s", stack.String())
	if err := stack.LogModuleDeployOrder(opts.Logger, opts.TerraformCommand); err != nil {
		return err
	}

	return stack.Run(ctx, opts.TerragruntOptions)
}
