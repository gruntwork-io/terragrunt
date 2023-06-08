package cli

// Since Terragrunt is just a thin wrapper for Terraform, and we don't want to repeat every single Terraform command
// in its definition, we don't quite fit into the model of any Go CLI library. Fortunately, urfave/cli allows us to
// override the whole template used for the Usage Text.
//
// TODO: this description text has copy/pasted versions of many Terragrunt constants, such as command names and file
// names. It would be easy to make this code DRY using fmt.Sprintf(), but then it's hard to make the text align nicely.
// Write some code to take generate this help text automatically, possibly leveraging code that's part of urfave/cli.
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

   terragrunt-config                            Path to the Terragrunt config file. Default is terragrunt.hcl.
   terragrunt-tfpath                            Path to the Terraform binary. Default is terraform (on PATH).
   terragrunt-no-auto-init                      Don't automatically run 'terraform init' during other terragrunt commands. You must run 'terragrunt init' manually.
   terragrunt-no-auto-retry                     Don't automatically re-run command in case of transient errors.
   terragrunt-non-interactive                   Assume "yes" for all prompts.
   terragrunt-working-dir                       The path to the Terraform templates. Default is current directory.
   terragrunt-download-dir                      The path where to download Terraform code. Default is .terragrunt-cache in the working directory.
   terragrunt-source                            Download Terraform configurations from the specified source into a temporary folder, and run Terraform in that temporary folder.
   terragrunt-source-update                     Delete the contents of the temporary folder to clear out any old, cached source code before downloading new source code into it.
   terragrunt-iam-role                          Assume the specified IAM role before executing Terraform. Can also be set via the TERRAGRUNT_IAM_ROLE environment variable.
   terragrunt-iam-assume-role-duration          Session duration for IAM Assume Role session. Can also be set via the TERRAGRUNT_IAM_ASSUME_ROLE_DURATION environment variable.
   terragrunt-iam-assume-role-session-name      Name for the IAM Assummed Role session. Can also be set via TERRAGRUNT_IAM_ASSUME_ROLE_SESSION_NAME environment variable.
   terragrunt-ignore-dependency-errors          *-all commands continue processing components even if a dependency fails.
   terragrunt-ignore-dependency-order           *-all commands will be run disregarding the dependencies
   terragrunt-ignore-external-dependencies      *-all commands will not attempt to include external dependencies
   terragrunt-include-external-dependencies     *-all commands will include external dependencies
   terragrunt-parallelism <N>                   *-all commands parallelism set to at most N modules
   terragrunt-exclude-dir                       Unix-style glob of directories to exclude when running *-all commands
   terragrunt-include-dir                       Unix-style glob of directories to include when running *-all commands
   terragrunt-check                             Enable check mode in the hclfmt command.
   terragrunt-hclfmt-file                       The path to a single hcl file that the hclfmt command should run on.
   terragrunt-override-attr                     A key=value attribute to override in a provider block as part of the aws-provider-patch command. May be specified multiple times.
   terragrunt-debug                             Write terragrunt-debug.tfvars to working folder to help root-cause issues.
   terragrunt-log-level                         Sets the logging level for Terragrunt. Supported levels: panic, fatal, error, warn (default), info, debug, trace.
   terragrunt-no-color                          If specified, output won't contain any color.
   terragrunt-strict-validate                   Sets strict mode for the validate-inputs command. By default, strict mode is off. When this flag is passed, strict mode is turned on. When strict mode is turned off, the validate-inputs command will only return an error if required inputs are missing from all input sources (env vars, var files, etc). When strict mode is turned on, an error will be returned if required inputs are missing OR if unused variables are passed to Terragrunt.
   terragrunt-json-out                          The file path that terragrunt should use when rendering the terragrunt.hcl config as json. Only used in the render-json command. Defaults to terragrunt_rendered.json.
   terragrunt-use-partial-parse-config-cache    Enables caching of includes during partial parsing operations. Will also be used for the --terragrunt-iam-role option if provided.
   terragrunt-include-module-prefix             When this flag is set output from Terraform sub-commands is prefixed with module path.

VERSION:
   {{.Version}}{{if len .Authors}}

AUTHOR(S):
   {{range .Authors}}{{.}}{{end}}
   {{end}}
`
