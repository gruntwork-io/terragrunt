package cli

// Since Terragrunt is just a thin wrapper for Terraform, and we don't want to repeat every single Terraform command
// in its definition, we don't quite fit into the model of any Go CLI library. Fortunately, urfave/cli allows us to
// override the whole template used for the Usage Text.
const appHelpTemplate = `USAGE: {{.Usage}}

DESCRIPTION:
   {{.Name}} - {{.UsageText}}

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
   {{end}}{{$option}}{{end}}{{if not .HideVersion}}

VERSION: {{.Version}}{{if len .Authors}}{{end}}

AUTHOR: {{range .Authors}}{{.}}{{end}} {{end}}
`

const renderJsonHelp = `USAGE: terragrunt render-json [OPTIONS]

DESCRIPTION:
   Render the final terragrunt config, with all variables, includes, and functions resolved, as json.

OPTIONS:
   --with-metadata 		Add metadata to the rendered JSON file.
   --terragrunt-json-out 	The file path that terragrunt should use when rendering the terragrunt.hcl config as json.{{if not .HideVersion}}

VERSION: {{.Version}}{{if len .Authors}}{{end}}

AUTHOR: {{range .Authors}}{{.}}{{end}} {{end}}
`

const awsProviderPatchHelp = `USAGE: terragrunt aws-provider-patch [OPTIONS]

DESCRIPTION:
   Overwrite settings on nested AWS providers to work around a Terraform bug (issue #13018)

OPTIONS:
   --terragrunt-override-attr	A key=value attribute to override in a provider block as part of the aws-provider-patch command. May be specified multiple times.{{if not .HideVersion}}

VERSION: {{.Version}}{{if len .Authors}}{{end}}

AUTHOR: {{range .Authors}}{{.}}{{end}} {{end}}
`

const validateInputsHelp = `USAGE: terragrunt validate-inputs [OPTIONS]

DESCRIPTION:
   Checks if the terragrunt configured inputs align with the terraform defined variables.

OPTIONS:
   --terragrunt-strict-validate		Enable strict mode for validation. When strict mode is turned on, an error will be returned if required inputs are missing OR if unused variables are passed to Terragrunt.{{if not .HideVersion}}

VERSION: {{.Version}}{{if len .Authors}}{{end}}

AUTHOR: {{range .Authors}}{{.}}{{end}} {{end}}
`
