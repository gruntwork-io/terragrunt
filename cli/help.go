package cli

const AppHelpTemplate = `NAME:
   {{$v := offset .App.HelpName 6}}{{wrap .App.HelpName 3}}{{if .App.Usage}} - {{wrap .App.Usage $v}}{{end}}

USAGE:
   {{if .App.UsageText}}{{wrap .App.UsageText 3}}{{else}}{{.App.HelpName}} <command> [options]{{end}} {{if .App.Description}}

DESCRIPTION:
   {{wrap .App.Description 3}}{{end}}{{if .App.VisibleCommands}}

COMMANDS:{{ $cv := offsetCommands .App.VisibleCommands 5}}{{range .App.VisibleCommands}}
   {{$s := .HelpName}}{{$s}}{{ $sp := subtract $cv (offset $s 3) }}{{ indent $sp ""}} {{wrap .Usage $cv}}{{end}}{{end}}

GLOBAL OPTIONS:{{if .Command.VisibleFlags}}
   {{range $index, $option := .App.VisibleFlags}}{{if $index}}
   {{end}}{{wrap $option.String 6}}{{end}}{{end}}{{if not .App.HideVersion}}

VERSION: {{.App.Version}}{{if len .App.Authors}}{{end}}

AUTHOR: {{range .App.Authors}}{{.}}{{end}} {{end}}
`

const CommandHelpTemplate = `NAME:
   {{$v := offset .Command.HelpName 6}}{{wrap .Command.HelpName 3}}{{if .Usage}} - {{wrap .Command.Usage $v}}{{end}}

USAGE:
   {{if .Command.UsageText}}{{wrap .Command.UsageText 3}}{{else}}terragrunt {{.Command.HelpName}}{{if .Command.VisibleSubcommands}} <command>{{end}}{{if .Command.VisibleFlags}} [options]{{end}}{{end}}{{if .Description}}

DESCRIPTION:
   {{wrap .Command.Description 3}}{{end}}{{if .Command.VisibleSubcommands}}

COMMANDS:{{ $cv := offsetCommands .Command.VisibleSubcommands 5}}{{range .Command.VisibleSubcommands}}
   {{$s := .HelpName}}{{$s}}{{ $sp := subtract $cv (offset $s 3) }}{{ indent $sp ""}} {{wrap .Usage $cv}}{{end}}{{end}}{{if .Command.VisibleFlags}}

OPTIONS:
   {{range $index, $option := .Command.VisibleFlags}}{{if $index}}
   {{end}}{{wrap $option.String 6}}{{end}}{{end}}{{if .App.VisibleFlags}}

GLOBAL OPTIONS:
   {{range $index, $option := .App.VisibleFlags}}{{if $index}}
   {{end}}{{wrap $option.String 6}}{{end}}{{end}}

`

const AppVersionTemplate = `terragrunt version {{.App.Version}}
`
