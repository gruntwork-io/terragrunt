package shell

import (
	"context"
	"errors"
	"io"
	"strings"

	"github.com/gruntwork-io/terragrunt/internal/os/exec"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/pkg/log"
)

// readLine reads up to and including the next newline, a byte at a time, and
// returns the line without it. Buffering the reads would be cheaper, but it
// would also pull in whatever follows the answer, and that read-ahead is
// unreachable afterwards: not to the next prompt, and not to a subprocess that
// inherits the same stream. A prompt is a handful of bytes typed by a person,
// so the syscall per byte costs nothing worth having.
//
// A final line that ends at EOF instead of a newline is returned as the answer,
// so a piped `printf yes` reads the same as a typed one.
func readLine(r io.Reader) (string, error) {
	var line []byte

	buf := make([]byte, 1)

	for {
		n, err := r.Read(buf)
		if n > 0 {
			if buf[0] == '\n' {
				return string(line), nil
			}

			line = append(line, buf[0])
		}

		if err != nil {
			if errors.Is(err, io.EOF) && len(line) > 0 {
				return string(line), nil
			}

			return "", err
		}
	}
}

// PromptUserForInput prompts the user for text in the CLI. Returns the text entered by the user.
func PromptUserForInput(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	prompt string,
	nonInteractive bool,
) (string, error) {
	v.RequireStdin()

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

	inputCh := make(chan string)
	errCh := make(chan error)

	go func() {
		input, err := readLine(v.Stdin)
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
