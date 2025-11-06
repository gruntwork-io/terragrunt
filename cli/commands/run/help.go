package run

import (
	"fmt"
	"io"
	"strings"

	"github.com/gruntwork-io/terragrunt/cli/flags/shared"
	"github.com/gruntwork-io/terragrunt/internal/cli"
	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/tf"
	"github.com/gruntwork-io/terragrunt/util"
)

// TFCommandHelpTemplate is the TF command CLI help template.
const TFCommandHelpTemplate = `Usage: {{ if .Command.UsageText }}{{ wrap .Command.UsageText 3 }}{{ else }}{{ range $parent := parentCommands . }}{{ $parent.HelpName }} {{ end }}[global options] {{ .Command.HelpName }} [options]{{ if eq .Command.Name "` + tf.CommandNameApply + `" }} [PLAN]{{ end }}{{ end }}{{ $description := .Command.Usage }}{{ if .Command.Description }}{{ $description = .Command.Description }}{{ end }}{{ if $description }}

   {{ wrap $description 3 }}{{ end }}{{ if ne .Parent.Command.Name "` + CommandName + `" }}

   This is a shortcut for the command ` + "`terragrunt " + CommandName + "`" + `.{{ end }}

   It wraps the ` + "`{{ tfCommand }}`" + ` command of the binary defined by ` + "`tf-path`" + `.

{{ if isTerraformPath }}Terraform{{ else }}OpenTofu{{ end }} ` + "`{{ tfCommand }}`" + ` help:{{ $tfHelp := runTFHelp }}{{ if $tfHelp }}

{{ $tfHelp }}{{ end }}
`

// ShowTFHelp prints TF help for the given `ctx.Command` command.
func ShowTFHelp(l log.Logger, opts *options.TerragruntOptions) cli.HelpFunc {
	return func(ctx *cli.Context) error {
		if err := shared.NewTFPathFlag(opts).Parse(ctx.Args()); err != nil {
			return err
		}

		cli.HelpPrinterCustom(ctx, TFCommandHelpTemplate, map[string]any{
			"isTerraformPath": func() bool {
				return isTerraformPath(opts)
			},
			"runTFHelp": func() string {
				return runTFHelp(ctx, l, opts)
			},
			"tfCommand": func() string {
				return ctx.Command.Name
			},
		})

		return nil
	}
}

func runTFHelp(ctx *cli.Context, l log.Logger, opts *options.TerragruntOptions) string {
	opts = opts.Clone()
	opts.Writer = io.Discard

	terraformHelpCmd := []string{tf.FlagNameHelpLong, ctx.Command.Name}

	out, err := tf.RunCommandWithOutput(ctx, l, opts, terraformHelpCmd...)
	if err != nil {
		var processError util.ProcessExecutionError
		if ok := errors.As(err, &processError); ok {
			err = processError.Err
		}

		return fmt.Sprintf("Failed to execute \"%s %s\": %s", opts.TFPath, strings.Join(terraformHelpCmd, " "), err.Error())
	}

	result := out.Stdout.String()
	lines := strings.Split(result, "\n")

	// Trim first empty lines or that has prefix "Usage:".
	for i := range lines {
		if strings.TrimSpace(lines[i]) == "" || strings.HasPrefix(lines[i], "Usage:") {
			continue
		}

		return strings.Join(lines[i:], "\n")
	}

	return result
}
