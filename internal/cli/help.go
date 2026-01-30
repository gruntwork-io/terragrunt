package cli

// AppHelpTemplate is the main CLI help template.
const AppHelpTemplate = `Usage: {{ if .App.UsageText }}{{ wrap .App.UsageText 3 }}{{ else }}{{ .App.HelpName }} [global options] <command> [options]{{ end }}{{ $description := .App.Usage }}{{ if .App.Description }}{{ $description = .App.Description }}{{ end }}{{ if $description }}

   {{ wrap $description 3 }}{{ end }}{{ $commands := .App.VisibleCommands }}{{ if $commands }}{{ $cv := offsetCommands $commands 5 }}
{{ $categories := $commands.GetCategories.Sort }}{{ range $index, $category := $categories }}{{ $categoryCommands := $commands.FilterByCategory $category }}{{ if $index }}
{{ end }}
{{ $category.Name }}:{{ range $categoryCommands }}
   {{ $s := .HelpName }}{{ $s }}{{ $sp := subtract $cv (offset $s 3) }}{{ indent $sp ""}} {{ wrap .Usage $cv }}{{ end }}{{ end }}{{ end }}{{ if .App.VisibleFlags }}

Global Options:
   {{ range $index, $option := .App.VisibleFlags }}{{ if $index }}
   {{ end }}{{ wrap $option.String 6 }}{{ end }}{{ end }}{{ if not .App.HideVersion }}

Version: {{ .App.Version }}{{ end }}{{ if len .App.Authors }}

Author: {{ range .App.Authors }}{{ . }}{{ end }} {{ end }}
`

// CommandHelpTemplate is the command CLI help template.
const CommandHelpTemplate = `Usage: {{ if .Command.UsageText }}{{ wrap .Command.UsageText 3 }}{{ else }}{{ range $index, $parent := parentCommands . }}{{ $parent.HelpName }} {{ end }}{{ .Command.HelpName }}{{ if .Command.VisibleSubcommands }} <command>{{ end }}{{ if .Command.VisibleFlags }} [options]{{ end }}{{ end }}{{ $description := .Command.Usage }}{{ if .Command.Description }}{{ $description = .Command.Description }}{{ end }}{{ if $description }}

   {{ wrap $description 3 }}{{ end }}{{ if .Command.Examples }}

Examples:
   {{ $s := join .Command.Examples "\n\n" }}{{ wrap $s 3 }}{{ end }}{{ if .Command.VisibleSubcommands }}

Commands:{{ $cv := offsetCommands .Command.VisibleSubcommands 5 }}{{ range .Command.VisibleSubcommands }}
   {{ $s := .HelpName }}{{ $s }}{{ $sp := subtract $cv (offset $s 3) }}{{ indent $sp ""}} {{ wrap .Usage $cv }}{{ end }}{{ end }}{{ if .Command.VisibleFlags }}

Options:
   {{ range $index, $option := .Command.VisibleFlags.Sort }}{{ if $index }}
   {{ end }}{{ wrap $option.String 6 }}{{ end }}{{ end }}{{ if .App.VisibleFlags }}

Global Options:
   {{ range $index, $option := .App.VisibleFlags }}{{ if $index }}
   {{ end }}{{ wrap $option.String 6 }}{{ end }}{{ end }}

`

const AppVersionTemplate = `{{ .App.Name }} version {{ .App.Version }}
`
