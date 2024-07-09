package hclvalidate

import (
	"context"

	"github.com/gruntwork-io/terragrunt/config"
	"github.com/gruntwork-io/terragrunt/config/hclparse"
	"github.com/gruntwork-io/terragrunt/configstack"
	"github.com/gruntwork-io/terragrunt/internal/view"
	"github.com/gruntwork-io/terragrunt/internal/view/diagnostic"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/hashicorp/hcl/v2"
)

func Run(ctx context.Context, opts *Options) (er error) {
	var diags diagnostic.Diagnostics

	parseOptions := []hclparse.Option{
		hclparse.WithDiagnosticsHandler(func(file *hcl.File, hclDiags hcl.Diagnostics) (hcl.Diagnostics, error) {
			for _, hclDiag := range hclDiags {
				if !diags.Contains(hclDiag) {
					newDiag := diagnostic.NewDiagnostic(file, hclDiag)
					diags = append(diags, newDiag)
				}
			}
			return nil, nil
		}),
	}

	opts.SkipOutput = true
	opts.NonInteractive = true
	opts.RunTerragrunt = func(ctx context.Context, opts *options.TerragruntOptions) error {
		_, err := config.ReadTerragruntConfig(ctx, opts, parseOptions)
		return err
	}

	stack, err := configstack.FindStackInSubfolders(ctx, opts.TerragruntOptions, configstack.WithParseOptions(parseOptions))
	if err != nil {
		return err
	}

	if err := stack.Run(ctx, opts.TerragruntOptions); err != nil {
		return nil
	}

	return writeDiagnostics(opts, diags)
}

func writeDiagnostics(opts *Options, diags diagnostic.Diagnostics) error {
	if len(diags) == 0 {
		return nil
	}

	render := view.NewHumanRender(opts.DisableLogColors)
	if opts.JSONOutput {
		render = view.NewJSONRender()
	}

	writer := view.NewWriter(opts.Writer, render)

	if opts.InvalidConfigPath {
		return writer.InvalidConfigPath(diags)
	}

	return writer.Diagnostics(diags)
}
