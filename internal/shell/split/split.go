// Package split consolidates Terragrunt's shell-style word splitting behind a
// single API, so callers pick a function that matches their use case rather
// than picking a library.
//
// # When to use what
//
// Use [Command] to split a command line Terragrunt runs or matches, e.g. an
// --auth-provider-cmd value or an --allow-cmd pattern. It follows POSIX shell
// quoting, expands no variables, runs no substitutions, and refuses an
// unquoted shell operator.
//
// Use [Quote] to render one word for a person to read as part of a command
// line.
//
// Use [LegacyCommand] only for call sites that participate in user-facing
// configuration surface (e.g. terraform.extra_arguments). It is backed by
// [shlex] and retained to avoid a silent behavior change for arguments users
// have written against it. Prefer [Command] for new code.
//
// [shlex]: https://pkg.go.dev/github.com/google/shlex
package split

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/google/shlex"
	"github.com/mattn/go-shellwords"
)

// ErrInvalidCommand is returned when a command line cannot be split, e.g. for
// an unterminated quote or an unquoted shell operator.
var ErrInvalidCommand = errors.New("invalid shell command line")

// ErrShellOperator is returned, wrapped in [ErrInvalidCommand], when a command
// line has an unquoted shell operator.
var ErrShellOperator = errors.New("unquoted shell operator, such as | or ;")

// Command splits a command line into words with POSIX shell quoting rules. A
// line with an unquoted shell operator, such as ; or |, fails with
// [ErrShellOperator].
//
// On Windows, each backslash is read as a forward slash, so a Windows path
// keeps its separators.
func Command(line string) ([]string, error) {
	parser := shellwords.NewParser()

	words, err := parser.Parse(filepath.ToSlash(line))
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalidCommand, err)
	}

	if parser.Position != -1 {
		return nil, fmt.Errorf("%w: %w", ErrInvalidCommand, ErrShellOperator)
	}

	return words, nil
}

// Quote renders word as one POSIX shell word, so a shell or [Command] reads it
// back as word, except that [Command] on Windows reads a backslash as a
// forward slash. A word made only of characters no shell treats specially is
// returned as is. Anything else is wrapped in single quotes, and each single
// quote inside it closes the quoting, appears backslash-escaped, and reopens
// it.
func Quote(word string) string {
	if word != "" && strings.IndexFunc(word, needsQuoting) == -1 {
		return word
	}

	return "'" + strings.ReplaceAll(word, "'", `'\''`) + "'"
}

// LegacyCommand splits a command line into words with the [shlex] grammar.
// Unlike [Command], it does not refuse shell operators or parentheses, and an
// unquoted word starting with # begins a comment that runs to the end of the
// line.
//
// [shlex]: https://pkg.go.dev/github.com/google/shlex
func LegacyCommand(line string) ([]string, error) {
	words, err := shlex.Split(line)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalidCommand, err)
	}

	return words, nil
}

// needsQuoting reports whether a shell could read r as anything but itself.
func needsQuoting(r rune) bool {
	switch {
	case 'a' <= r && r <= 'z', 'A' <= r && r <= 'Z', '0' <= r && r <= '9':
		return false
	}

	return !strings.ContainsRune("%+,-./:=@_", r)
}
