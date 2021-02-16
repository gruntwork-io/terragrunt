package shell

import (
	"bufio"
	"fmt"
	"github.com/gruntwork-io/terragrunt/errors"
	"github.com/gruntwork-io/terragrunt/options"
	"os"
	"strings"
)

// Prompt the user for text in the CLI. Returns the text entered by the user.
func PromptUserForInput(prompt string, terragruntOptions *options.TerragruntOptions) (string, error) {
	// We are writing directly to ErrWriter so the prompt is always visible
	// no matter what logLevel is configured. If `--non-interactive` is set, we return "yes" early,
	// so that prompt is not shown
	//
	// See https://github.com/gruntwork-io/terragrunt/issues/1524 for additional context
	if terragruntOptions.NonInteractive {
		terragruntOptions.Logger.Infof("The non-interactive flag is set to true, so assuming 'yes' for all prompts")
		return "yes", nil
	}
	n, err := terragruntOptions.ErrWriter.Write([]byte(prompt))
	if err != nil {
		terragruntOptions.Logger.Error(err)
		return "", errors.WithStackTrace(err)
	}
	if n != len(prompt) {
		terragruntOptions.Logger.Errorln("Failed to write data")
		return "", errors.WithStackTrace(err)
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
