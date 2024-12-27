// Package placeholders represents a set of placeholders for formatting various log values.
package placeholders

import (
	"strings"

	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/pkg/log/format/options"
)

const (
	placeholderSign             = "%"
	splitIntoTextAndPlaceholder = 2
)

// Placeholder is part of the log message, used to format different log values.
type Placeholder interface {
	// Name returns a placeholder name.
	Name() string
	// Options returns the placeholder options.
	Options() options.Options
	// Format returns the formatted value.
	Format(data *options.Data) (string, error)
}

// Placeholders are a set of Placeholders.
type Placeholders []Placeholder

// NewPlaceholderRegister returns a new `Placeholder` collection instance available for use in a custom format string.
func NewPlaceholderRegister() Placeholders {
	return Placeholders{
		Interval(),
		Time(),
		Level(),
		Message(),
		Field(WorkDirKeyName, options.PathFormat(options.NonePath, options.RelativePath, options.ShortRelativePath, options.ShortPath)),
		Field(TFPathKeyName, options.PathFormat(options.NonePath, options.FilenamePath, options.DirectoryPath)),
		Field(TFCmdArgsKeyName),
		Field(TFCmdKeyName),
	}
}

// Get returns the placeholder by its name.
func (phs Placeholders) Get(name string) Placeholder {
	for _, ph := range phs {
		if ph.Name() == name {
			return ph
		}
	}

	return nil
}

// Names returns the names of the placeholders.
func (phs Placeholders) Names() []string {
	var names = make([]string, len(phs))

	for i, ph := range phs {
		names[i] = ph.Name()
	}

	return names
}

// Format returns a formatted string that is the concatenation of the formatted placeholder values.
func (phs Placeholders) Format(data *options.Data) (string, error) {
	var str string

	for _, ph := range phs {
		s, err := ph.Format(data)
		if err != nil {
			return "", err
		}

		str += s
	}

	return str, nil
}

// findPlaceholder parses the given `str` to find a placeholder name present in the `phs` collection,
// returns that placeholder, and the rest of the given `str`.
//
// e.g. "level(color=green, case=upper) some-text" returns the instance of the `level` placeholder
// and "(color=green, case=upper) some-text" string.
func (phs Placeholders) findPlaceholder(str string) (Placeholder, string) { //nolint:ireturn
	var (
		placeholder Placeholder
		optIndex    int
	)

	// We don't stop at the first one we find, we look for the longest name.
	// Of these two `%tf-command` `%tf-command-args` we need to find the second one.
	for index := range len(str) {
		if !isPlaceholderNameCharacter(str[index]) {
			break
		}

		name := str[:index+1]

		if pl := phs.Get(name); pl != nil {
			placeholder = pl
			optIndex = index + 1
		}
	}

	if placeholder != nil {
		return placeholder, str[optIndex:]
	}

	return findPlaintextPlaceholder(str)
}

// Parse parses the given `str` and returns a set of placeholders that are then used to format log data.
func Parse(str string) (Placeholders, error) {
	var (
		placeholders Placeholders
		placeholder  Placeholder
		err          error
	)

	for {
		// We need to create a new placeholders collection to avoid overriding options
		// if the custom format string contains two or more same placeholders.
		// e.g. "%level(format=full) some-text %level(format=tiny)"
		placeholderRegister := NewPlaceholderRegister()

		parts := strings.SplitN(str, placeholderSign, splitIntoTextAndPlaceholder)

		if plaintext := parts[0]; plaintext != "" {
			placeholders = append(placeholders, PlainText(plaintext))
		}

		if len(parts) == 1 {
			return placeholders, nil
		}

		str = parts[1]

		placeholder, str = placeholderRegister.findPlaceholder(str)
		if placeholder == nil {
			return nil, errors.New(NewInvalidPlaceholderNameError(str, placeholderRegister))
		}

		str, err = placeholder.Options().Configure(str)
		if err != nil {
			return nil, errors.New(NewInvalidPlaceholderOptionError(placeholder, err))
		}

		placeholders = append(placeholders, placeholder)
	}
}

func findPlaintextPlaceholder(str string) (Placeholder, string) { //nolint:ireturn
	if len(str) == 0 {
		return nil, str
	}

	switch str[0:1] {
	case options.OptStartSign:
		// Unnamed placeholder, format `%(content='...')`.
		return PlainText(""), str
	case " ":
		// Single `%` character, format `% `.
		return PlainText(placeholderSign), str
	case placeholderSign:
		// Escaped `%`, format `%%`.
		return PlainText(placeholderSign), str[1:]
	case "t":
		// Indent, format `%t`.
		return PlainText("\t"), str[1:]
	case "n":
		// Newline, format `%n`.
		return PlainText("\n"), str[1:]
	}

	return nil, str
}

// isPlaceholderNameCharacter returns true if the given character `c` does not contain any restricted characters for placeholder names.
//
// e.g. "time" return `true`.
// e.g. "time " return `false`.
// e.g. "time(" return `false`.
func isPlaceholderNameCharacter(c byte) bool {
	// Check if the byte value falls within the range of alphanumeric characters
	return c == '-' || c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9')
}
