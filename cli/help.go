package cli

const AppHelpTemplate = `NAME:
   {{$v := offset .App.HelpName 6}}{{wrap .App.HelpName 3}}{{if .App.Usage}} - {{wrap .App.Usage $v}}{{end}}

USAGE:
   {{if .App.UsageText}}{{wrap .App.UsageText 3}}{{else}}{{.App.HelpName}} <command> [options]{{end}} {{if .App.Description}}

DESCRIPTION:
   {{wrap .App.Description 3}}{{end}}{{if .App.VisibleCommands}}

COMMANDS:{{ $cv := offsetCommands .App.VisibleCommands 5}}{{range .App.VisibleCommands}}
   {{$s := .HelpName}}{{$s}}{{ $sp := subtract $cv (offset $s 3) }}{{ indent $sp ""}} {{wrap .Usage $cv}}{{end}}{{end}}

GLOBAL OPTIONS:{{if .App.VisibleFlags}}
   {{range $index, $option := .App.VisibleFlags}}{{if $index}}
   {{end}}{{wrap $option.String 6}}{{end}}{{end}}{{if not .App.HideVersion}}

VERSION: {{.App.Version}}{{if len .App.Authors}}{{end}}

AUTHOR: {{range .App.Authors}}{{.}}{{end}} {{end}}
`

const CommandHelpTemplate = `Usage: {{if .Command.UsageText}}{{wrap .Command.UsageText 3}}{{else}}{{range $parent := parentCommands . }}{{$parent.HelpName}} {{end}}{{.Command.HelpName}}{{if .Command.VisibleSubcommands}} <command>{{end}}{{if .Command.VisibleFlags}} [options]{{end}}{{end}}{{$description := .Command.Usage}}{{if .Command.Description}}{{$description = .Command.Description}}{{end}}{{if $description}}

   {{wrap $description 3}}{{end}}{{if .Command.Examples}}

Examples:
   {{$s := join .Command.Examples "\n\n"}}{{wrap $s 3}}{{end}}{{if .Command.VisibleSubcommands}}

Subcommands:{{ $cv := offsetCommands .Command.VisibleSubcommands 5}}{{range .Command.VisibleSubcommands}}
   {{$s := .HelpName}}{{$s}}{{ $sp := subtract $cv (offset $s 3) }}{{ indent $sp ""}} {{wrap .Usage $cv}}{{end}}{{end}}{{if .Command.VisibleFlags}}

Options:
   {{range $index, $option := .Command.VisibleFlags}}{{if $index}}
   {{end}}{{wrap $option.String 6}}{{end}}{{end}}{{if .App.VisibleFlags}}

Global Options:
   {{range $index, $option := .App.VisibleFlags}}{{if $index}}
   {{end}}{{wrap $option.String 6}}{{end}}{{end}}

`

const AppVersionTemplate = `{{.App.Name}} version {{.App.Version}}
`
