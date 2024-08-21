package hclvalidate

import (
	"context"
	"sort"

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
				newDiag := diagnostic.NewDiagnostic(file, hclDiag)
				if !diags.Contains(newDiag) {
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

	stackErr := stack.Run(ctx, opts.TerragruntOptions)

	if len(diags) > 0 {
		sort.Slice(diags, func(i, j int) bool {
			if diags[i].Range != nil && diags[j].Range != nil && diags[i].Range.Filename > diags[j].Range.Filename {
				return false
			}

			return true
		})

		if err := writeDiagnostics(opts, diags); err != nil {
			return err
		}
	}

	return stackErr
}

func writeDiagnostics(opts *Options, diags diagnostic.Diagnostics) error {
	render := view.NewHumanRender(opts.DisableLogColors)
	if opts.JSONOutput {
		render = view.NewJSONRender()
	}

	writer := view.NewWriter(opts.Writer, render)

	if opts.ShowConfigPath {
		return writer.ShowConfigPath(diags)
	}

	return writer.Diagnostics(diags)
}
