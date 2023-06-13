package cli

import (
	"github.com/urfave/cli/v2"
)

type Commands []*Command

// Get returns a Command by the given name.
func (commands Commands) Get(name string) *Command {
	for _, command := range commands {
		if command.HasName(name) {
			return command
		}
	}

	return nil
}

// Filter returns a list of commands filtered by the given names.
func (commands Commands) Filter(names []string) Commands {
	var filtered Commands

	for _, command := range commands {
		for _, name := range names {
			if command.HasName(name) {
				filtered = append(filtered, command)
			}
		}
	}

	return filtered
}

// VisibleCommands returns a slice of the Commands with Hidden=false.
// Used by `urfave/cli` package to generate help.
func (commands Commands) VisibleCommands() []*cli.Command {
	var visible []*cli.Command

	for _, command := range commands {
		if command.Hidden {
			continue
		}

		visible = append(visible, &cli.Command{
			Name:        command.Name,
			Aliases:     command.Aliases,
			HelpName:    command.HelpName,
			Usage:       command.Usage,
			UsageText:   command.UsageText,
			Description: command.Description,
			Hidden:      command.Hidden,
		})
	}

	return visible
}

func (commands Commands) Len() int {
	return len(commands)
}

func (commands Commands) Less(i, j int) bool {
	return lexicographicLess(commands[i].Name, commands[j].Name)
}

func (commands Commands) Swap(i, j int) {
	commands[i], commands[j] = commands[j], commands[i]
}

func (commands Commands) parseArgs(args []string, dryRun bool) (*Command, []string, error) {
	var name string
	var undefArg []string

	for {
		if len(args) == 0 {
			return nil, nil, nil
		}
		name, args = args[0], args[1:]

		if command := commands.Get(name); command != nil {
			if dryRun {
				return command, append(undefArg, args...), nil
			}

			command, args, err := command.parseArgs(args)
			return command, append(undefArg, args...), err
		}

		undefArg = append(undefArg, name)
	}
}
