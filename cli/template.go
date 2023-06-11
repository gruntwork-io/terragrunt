package cli

// Since Terragrunt is just a thin wrapper for Terraform, and we don't want to repeat every single Terraform command
// in its definition, we don't quite fit into the model of any Go CLI library. Fortunately, urfave/cli allows us to
// override the whole template used for the Usage Text.
const appHelpTemplate = `NAME:
   {{$v := offset .HelpName 6}}{{wrap .HelpName 3}}{{if .Usage}} - {{wrap .Usage $v}}{{end}}

USAGE:
   {{if .UsageText}}{{wrap .UsageText 3}}{{else}}{{.HelpName}} {{if .VisibleFlags}}[global options]{{end}}{{if .Commands}} command [command options]{{end}} {{if .ArgsUsage}}{{.ArgsUsage}}{{else}}[arguments...]{{end}}{{end}}{{if .Description}}

DESCRIPTION:
   {{wrap .Description 3}}{{end}}

COMMANDS:{{ $cv := offsetCommands .VisibleCommands 5}}{{range .VisibleCommands}}
   {{$s := join .Names ", "}}{{$s}}{{ $sp := subtract $cv (offset $s 3) }}{{ indent $sp ""}}{{wrap .Usage $cv}}{{end}}

GLOBAL OPTIONS:
   {{range $index, $option := .VisibleFlags}}{{if $index}}
   {{end}}{{$option}}{{end}}{{if not .HideVersion}}

VERSION: {{.Version}}{{if len .Authors}}{{end}}

AUTHOR: {{range .Authors}}{{.}}{{end}} {{end}}
`

const commandHelpTemplate = `NAME:
   {{$v := offset .HelpName 6}}{{wrap .HelpName 3}}{{if .Usage}} - {{wrap .Usage $v}}{{end}}

USAGE:
   terragrunt [global options] {{if .UsageText}}{{wrap .UsageText 3}}{{else}}{{.HelpName}}{{if .VisibleFlags}} [command options]{{end}}{{end}}{{if .Description}}

DESCRIPTION:
   {{wrap .Description 3}}{{end}}{{if .VisibleFlags}}

OPTIONS:{{range $i, $e := .VisibleFlags}}
   {{wrap $e.String 6}}{{end}}{{end}}

`
