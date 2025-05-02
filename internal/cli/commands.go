package cli

import (
	"sort"
	"strings"

	"slices"
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

// Names returns names of the commands.
func (commands Commands) Names() []string {
	var names = make([]string, len(commands))

	for i, cmd := range commands {
		names[i] = cmd.Name
	}

	return names
}

// Add adds a new cmd to the list.
func (commands *Commands) Add(cmd *Command) {
	*commands = append(*commands, cmd)
}

// FilterByNames returns a list of commands filtered by the given names.
func (commands Commands) FilterByNames(names []string) Commands {
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

// FilterByCategory returns a list of commands filtered by the given `categories`.
func (commands Commands) FilterByCategory(categories ...*Category) Commands {
	var filtered Commands

	for _, cmd := range commands {
		if category := cmd.Category; category != nil && slices.Contains(categories, category) {
			filtered = append(filtered, cmd)
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
func (commands Commands) VisibleCommands() Commands {
	var visible = make(Commands, 0, len(commands))

	for _, cmd := range commands {
		if cmd.Hidden {
			continue
		}

		if cmd.HelpName == "" {
			names := append([]string{cmd.Name}, cmd.Aliases...)

			cmd.HelpName = strings.Join(names, ", ")
		}

		visible = append(visible, cmd)
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

func (commands Commands) WrapAction(fn func(ctx *Context, action ActionFunc) error) Commands {
	wrapped := make(Commands, len(commands))

	for i := range commands {
		wrapped[i] = commands[i].WrapAction(fn)
	}

	return wrapped
}

func (commands Commands) Sort() Commands {
	sort.Sort(commands)

	return commands
}

// SetCategory sets the given `category` for the `commands`.
func (commands Commands) SetCategory(category *Category) Commands {
	for _, cmd := range commands {
		cmd.Category = category
	}

	return commands
}

// GetCategories returns unique categories commands.
func (commands Commands) GetCategories() Categories {
	var categories Categories

	for _, cmd := range commands {
		if category := cmd.Category; category != nil && !slices.Contains(categories, category) {
			categories = append(categories, category)
		}
	}

	return categories
}

// Merge merges the given `cmds` with `commands` and returns the result.
func (commands Commands) Merge(cmds ...*Command) Commands {
	return append(commands, cmds...)
}

// DisableErrorOnMultipleSetFlag returns a cloned command with disabled the check for multiple values set for the same flag.
func (commands Commands) DisableErrorOnMultipleSetFlag() Commands {
	var newCommands = make(Commands, len(commands))

	for i := range commands {
		newCommands[i] = commands[i].DisableErrorOnMultipleSetFlag()
	}

	return newCommands
}
