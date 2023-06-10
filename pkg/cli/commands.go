package cli

import (
	"github.com/gruntwork-io/go-commons/collections"
)

type Commands []*Command

// Get returns a Command by the given name.
func (commands Commands) Get(name string) *Command {
	for _, command := range commands {
		if collections.ListContainsElement(command.Names(), name) {
			return command
		}
	}

	return nil
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
