package shell

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/gruntwork-io/terragrunt/errors"
	"github.com/gruntwork-io/terragrunt/options"
	"golang.org/x/crypto/ssh/terminal"
)

// CanAcceptPrompt tells whether the program can receive input on the standard
// input.
func CanAcceptPrompt() bool {
	return terminal.IsTerminal(int(os.Stdin.Fd()))
}

// Prompt the user for text in the CLI. Returns the text entered by the user.
func PromptUserForInput(prompt string, terragruntOptions *options.TerragruntOptions) (string, error) {
	if terragruntOptions.Logger.Prefix() != "" {
		prompt = fmt.Sprintf("%s %s", terragruntOptions.Logger.Prefix(), prompt)
	}
	terragruntOptions.Logger.Print(prompt)

	if terragruntOptions.NonInteractive {
		terragruntOptions.Logger.Println()
		terragruntOptions.Logger.Printf("The non-interactive flag is set to true, so assuming 'yes' for all prompts")
		return "yes", nil
	}

	reader := bufio.NewReader(os.Stdin)

	text, err := reader.ReadString('\n')
	if err != nil {
		return "", errors.WithStackTrace(err)
	}

	return strings.TrimSpace(text), nil
}

// Prompt the user for a yes/no response and return true if they entered yes.
func PromptUserForYesNo(prompt string, terragruntOptions *options.TerragruntOptions) (bool, error) {
	resp, err := PromptUserForInput(fmt.Sprintf("%s (y/n) ", prompt), terragruntOptions)

	if err != nil {
		return false, errors.WithStackTrace(err)
	}

	switch strings.ToLower(resp) {
	case "y", "yes":
		return true, nil
	default:
		return false, nil
	}
}
