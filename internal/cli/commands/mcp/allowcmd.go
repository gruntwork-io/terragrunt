package mcp

import (
	"errors"
	"fmt"
	"strings"

	"github.com/gruntwork-io/terragrunt/internal/glob"
	"github.com/gruntwork-io/terragrunt/internal/shell/split"
)

// ErrInvalidAllowCmd is returned when an --allow-cmd pattern cannot be read as
// a program name followed by argument patterns.
var ErrInvalidAllowCmd = errors.New("invalid --" + AllowCmdFlagName + " pattern")

// allowCmdAnyWords is the pattern word that matches any number of argument
// words, none included.
const allowCmdAnyWords = "**"

// AllowCmd is one parsed --allow-cmd pattern. Read one with [ParseAllowCmd].
type AllowCmd struct {
	// program is the program the pattern names, reduced by execProgram.
	program string
	// runs are the argument words between ** words, in order. A pattern with
	// no ** word has exactly one run.
	runs [][]glob.Matcher
}

// ParseAllowCmd reads one --allow-cmd pattern into a matcher, splitting it into
// words with shell quoting rules (see [split.Command]).
//
// The first word names the program as a bare name or a path, and matches it
// exactly once both are reduced the way [AllowCmd.Matches] describes. It may
// not contain glob syntax, so a pattern never widens which program runs. Each
// later word matches one argument word, with glob syntax such as * applying
// within that word. The word ** matches any number of argument words, none
// included.
func ParseAllowCmd(pattern string) (AllowCmd, error) {
	words, err := split.Command(pattern)
	if err != nil {
		return AllowCmd{}, fmt.Errorf("%w %q: %w", ErrInvalidAllowCmd, pattern, err)
	}

	if len(words) == 0 || words[0] == "" || strings.ContainsAny(words[0], `*?[]{}`) {
		return AllowCmd{}, fmt.Errorf(
			"%w %q: start with the name or path of a program, without glob syntax",
			ErrInvalidAllowCmd,
			pattern,
		)
	}

	cmd := AllowCmd{
		program: execProgram(words[0]),
		runs:    [][]glob.Matcher{nil},
	}

	for _, word := range words[1:] {
		if word == allowCmdAnyWords {
			cmd.runs = append(cmd.runs, nil)

			continue
		}

		m, err := glob.Compile(word, glob.WithoutSeparator())
		if err != nil {
			return AllowCmd{}, fmt.Errorf("%w %q: argument %q: %w", ErrInvalidAllowCmd, pattern, word, err)
		}

		last := len(cmd.runs) - 1
		cmd.runs[last] = append(cmd.runs[last], m)
	}

	return cmd, nil
}

// Matches reports whether the pattern allows running program with args.
//
// A bare program name matches the same name, with any .exe suffix removed on
// either side. A program named by a path matches the same path once both are
// lexically cleaned, so ./scripts/version.sh and scripts/version.sh are one
// program. A bare name never matches a path, and a relative path never matches
// an absolute one.
func (c AllowCmd) Matches(program string, args []string) bool {
	if execProgram(program) != c.program {
		return false
	}

	first := c.runs[0]
	if len(c.runs) == 1 {
		return runMatches(args, first)
	}

	last := c.runs[len(c.runs)-1]
	if len(first)+len(last) > len(args) ||
		!runMatches(args[:len(first)], first) ||
		!runMatches(args[len(args)-len(last):], last) {
		return false
	}

	rest := args[len(first) : len(args)-len(last)]

	for _, run := range c.runs[1 : len(c.runs)-1] {
		i := indexRun(rest, run)
		if i < 0 {
			return false
		}

		rest = rest[i+len(run):]
	}

	return true
}

// parseAllowCmds reads every --allow-cmd pattern, and fails on the first one
// it cannot read.
func parseAllowCmds(patterns []string) ([]AllowCmd, error) {
	cmds := make([]AllowCmd, 0, len(patterns))

	for _, pattern := range patterns {
		cmd, err := ParseAllowCmd(pattern)
		if err != nil {
			return nil, err
		}

		cmds = append(cmds, cmd)
	}

	return cmds, nil
}

// indexRun returns the index of the first place in args where run matches, or
// -1 when it matches nowhere. Taking the first place leaves the most arguments
// for the runs after it.
func indexRun(args []string, run []glob.Matcher) int {
	for i := range len(args) - len(run) + 1 {
		if runMatches(args[i:i+len(run)], run) {
			return i
		}
	}

	return -1
}

// runMatches reports whether args has one argument per word in run, each
// matching its word.
func runMatches(args []string, run []glob.Matcher) bool {
	if len(args) != len(run) {
		return false
	}

	for i, m := range run {
		if !m.Match(args[i]) {
			return false
		}
	}

	return true
}
