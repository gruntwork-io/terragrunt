package cli

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

// VisibleCommands returns a slice of the Commands with Hidden=false
func (commands Commands) VisibleCommands() Commands {
	var visible Commands
	for _, command := range commands {
		if !command.Hidden {
			visible = append(visible, command)
		}
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

func (commands Commands) parseArgs(args []string) (*Command, []string, error) {
	if len(args) == 0 {
		return nil, nil, nil
	}
	name, args := args[0], args[1:]

	if command := commands.Get(name); command != nil {
		args, err := command.parseArgs(args)
		return command, args, err
	}

	return nil, nil, nil
}
