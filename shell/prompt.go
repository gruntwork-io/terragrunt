package shell

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/gruntwork-io/go-commons/errors"
	"github.com/gruntwork-io/terragrunt/options"
)

// Prompt the user for text in the CLI. Returns the text entered by the user.
func PromptUserForInput(prompt string, terragruntOptions *options.TerragruntOptions) (string, error) {
	// We are writing directly to ErrWriter so the prompt is always visible
	// no matter what logLevel is configured. If `--non-interactive` is set, we log both prompt and
	// a message about assuming `yes` to Debug, so
	if terragruntOptions.NonInteractive {
		terragruntOptions.Logger.Debugf(prompt)
		terragruntOptions.Logger.Debugf("The non-interactive flag is set to true, so assuming 'yes' for all prompts")
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
