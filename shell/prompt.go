package shell

import (
	"bufio"
	"context"
	"os"
	"strings"

	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/options"
)

// PromptUserForInput prompts the user for text in the CLI. Returns the text entered by the user.
func PromptUserForInput(ctx context.Context, prompt string, terragruntOptions *options.TerragruntOptions) (string, error) {
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

		return "", errors.New(err)
	}

	if n != len(prompt) {
		terragruntOptions.Logger.Errorln("Failed to write data")

		return "", errors.New(err)
	}

	reader := bufio.NewReader(os.Stdin)

	inputCh := make(chan string)
	errCh := make(chan error)

	go func() {
		input, err := reader.ReadString('\n')
		if err != nil {
			errCh <- errors.New(err)
			return
		}
		inputCh <- strings.TrimSpace(input)
	}()

	select {
	case <-ctx.Done():
		return "", ctx.Err()
	case err := <-errCh:
		return "", err
	case input := <-inputCh:
		return input, nil
	}
}

// PromptUserForYesNo prompts the user for a yes/no response and return true if they entered yes.
func PromptUserForYesNo(ctx context.Context, prompt string, terragruntOptions *options.TerragruntOptions) (bool, error) {
	resp, err := PromptUserForInput(ctx, prompt+" (y/n) ", terragruntOptions)
	if err != nil {
		return false, errors.New(err)
	}

	switch strings.ToLower(resp) {
	case "y", "yes":
		return true, nil
	default:
		return false, nil
	}
}
