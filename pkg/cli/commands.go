package cli

import (
	"github.com/urfave/cli/v2"
)

type Commands []*Command

// Get returns a Command by the given name.
func (commands Commands) Get(name string) *Command {
	for _, cmd := range commands {
		if cmd.HasName(name) {
			return cmd
		}
	}

	return nil
}

// Add adds a new cmd to the list.
func (commands *Commands) Add(cmd *Command) {
	*commands = append(*commands, cmd)
}

// Filter returns a list of commands filtered by the given names.
func (commands Commands) Filter(names []string) Commands {
	var filtered Commands

	for _, cmd := range commands {
		for _, name := range names {
			if cmd.HasName(name) {
				filtered = append(filtered, cmd)
			}
		}
	}

	return filtered
}

// SkipRunning prevents running commands as the final commands, but keep showing them in help.
func (commands Commands) SkipRunning() Commands {
	for _, cmd := range commands {
		cmd.SkipRunning = true
	}

	return commands
}

// VisibleCommands returns a slice of the Commands with Hidden=false.
// Used by `urfave/cli` package to generate help.
func (commands Commands) VisibleCommands() []*cli.Command {
	var visible []*cli.Command

	for _, cmd := range commands {
		if cmd.Hidden {
			continue
		}

		if cmd.HelpName == "" {
			cmd.HelpName = cmd.Name
		}

		visible = append(visible, &cli.Command{
			Name:        cmd.Name,
			Aliases:     cmd.Aliases,
			HelpName:    cmd.HelpName,
			Usage:       cmd.Usage,
			UsageText:   cmd.UsageText,
			Description: cmd.Description,
			Hidden:      cmd.Hidden,
		})
	}

	return visible
}

func (commands Commands) Len() int {
	return len(commands)
}

func (commands Commands) Less(i, j int) bool {
	return LexicographicLess(commands[i].Name, commands[j].Name)
}

func (commands Commands) Swap(i, j int) {
	commands[i], commands[j] = commands[j], commands[i]
}
