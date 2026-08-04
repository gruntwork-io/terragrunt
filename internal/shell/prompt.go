package shell

import (
	"bufio"
	"context"
	"strings"

	"github.com/gruntwork-io/terragrunt/internal/os/exec"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/pkg/log"
)

// PromptUserForInput prompts the user for text in the CLI. Returns the text entered by the user.
func PromptUserForInput(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	prompt string,
	nonInteractive bool,
) (string, error) {
	errWriter := v.Writers.ErrWriter

	// We are writing directly to ErrWriter so the prompt is always visible
	// no matter what logLevel is configured. If `--non-interactive` is set, we log both prompt and
	// a message about assuming `yes` to Debug, so
	if nonInteractive {
		l.Debugf("%s", prompt)
		l.Debugf("The non-interactive flag is set to true, so assuming 'yes' for all prompts")

		return "yes", nil
	}

	n, err := errWriter.Write([]byte(prompt))
	if err != nil {
		l.Error(err)

		return "", err
	}

	if n != len(prompt) {
		l.Errorln("Failed to write data")

		return "", err
	}

	exec.PrepareStdinForPrompt(l)

	reader := bufio.NewReader(v.Reader)

	inputCh := make(chan string)
	errCh := make(chan error)

	go func() {
		input, err := reader.ReadString('\n')
		if err != nil {
			errCh <- err
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
func PromptUserForYesNo(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	prompt string,
	nonInteractive bool,
) (bool, error) {
	resp, err := PromptUserForInput(ctx, l, v, prompt+" (y/n) ", nonInteractive)
	if err != nil {
		return false, err
	}

	switch strings.ToLower(resp) {
	case "y", "yes":
		return true, nil
	default:
		return false, nil
	}
}
