package cli

// Since Terragrunt is just a thin wrapper for Terraform, and we don't want to repeat every single Terraform command
// in its definition, we don't quite fit into the model of any Go CLI library. Fortunately, urfave/cli allows us to
// override the whole template used for the Usage Text.
const AppHelpTemplate = `DESCRIPTION:
   {{.Name}} - {{.UsageText}}

USAGE:
   {{.Usage}}

COMMANDS:
   run-all               Run a terraform command against a 'stack' by running the specified command in each subfolder. E.g., to run 'terragrunt apply' in each subfolder, use 'terragrunt run-all apply'.
   terragrunt-info       Emits limited terragrunt state on stdout and exits
   validate-inputs       Checks if the terragrunt configured inputs align with the terraform defined variables.
   graph-dependencies    Prints the terragrunt dependency graph to stdout
   hclfmt                Recursively find hcl files and rewrite them into a canonical format.
   aws-provider-patch    Overwrite settings on nested AWS providers to work around a Terraform bug (issue #13018)
   render-json           Render the final terragrunt config, with all variables, includes, and functions resolved, as json. This is useful for enforcing policies using static analysis tools like Open Policy Agent, or for debugging your terragrunt config.
   *                     Terragrunt forwards all other commands directly to Terraform

GLOBAL OPTIONS:
   {{range $index, $option := .VisibleFlags}}{{if $index}}
   {{end}}{{$option}}{{end}}

VERSION:
   {{.Version}}{{if len .Authors}}

AUTHOR(S):
   {{range .Authors}}{{.}}{{end}}
   {{end}}
`
