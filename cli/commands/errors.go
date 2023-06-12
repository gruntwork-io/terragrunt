package commands

type MissingCommand struct{}

func (commandName MissingCommand) Error() string {
	return "Missing run-all command argument (Example: terragrunt run-all plan)"
}
