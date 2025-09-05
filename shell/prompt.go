package shell

import (
	"bufio"
	"context"
	"os"
	"strings"

	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/pkg/log"
)

// PromptUserForInput prompts the user for text in the CLI. Returns the text entered by the user.
func PromptUserForInput(ctx context.Context, l log.Logger, prompt string, opts *options.TerragruntOptions) (string, error) {
	// We are writing directly to ErrWriter so the prompt is always visible
	// no matter what logLevel is configured. If `--non-interactive` is set, we log both prompt and
	// a message about assuming `yes` to Debug, so
	if opts.NonInteractive {
		l.Debugf(prompt)
		l.Debugf("The non-interactive flag is set to true, so assuming 'yes' for all prompts")

		return "yes", nil
	}

	n, err := opts.ErrWriter.Write([]byte(prompt))
	if err != nil {
		l.Error(err)

		return "", errors.New(err)
	}

	if n != len(prompt) {
		l.Errorln("Failed to write data")

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
func PromptUserForYesNo(ctx context.Context, l log.Logger, prompt string, opts *options.TerragruntOptions) (bool, error) {
	resp, err := PromptUserForInput(ctx, l, prompt+" (y/n) ", opts)
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
