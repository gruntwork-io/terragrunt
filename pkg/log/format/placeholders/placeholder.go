// Package placeholders represents a set of placeholders for formatting various log values.
package placeholders

import (
	"strings"

	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/pkg/log/format/options"
)

const placeholderSign = "%"

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

func newPlaceholders() Placeholders {
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

// Parse parses the given string and returns a set of placeholders that are then used to format log data.
func Parse(str string) (Placeholders, error) {
	var (
		splitIntoTextAndPlaceholder = 2
		parts                       = strings.SplitN(str, placeholderSign, splitIntoTextAndPlaceholder)
		plaintext                   = parts[0]
		placeholders                = Placeholders{PlainText(plaintext)}
	)

	if len(parts) == 1 {
		return placeholders, nil
	}

	if strings.HasPrefix(parts[1], placeholderSign) {
		// `%%` evaluates as `%`.
		placeholders = append(placeholders, PlainText(placeholderSign))

		phs, err := Parse(str)
		if err != nil {
			return nil, err
		}

		return append(placeholders, phs...), nil
	}

	str = parts[1]
	if str == "" {
		return nil, errors.Errorf("empty placeholder name")
	}

	registered := newPlaceholders()
	placeholder, str := registered.parsePlaceholder(str)

	if placeholder == nil {
		return nil, errors.Errorf("invalid placeholder name %q, available names: %s", str, strings.Join(registered.Names(), ","))
	}

	str, err := placeholder.Options().Parse(str)
	if err != nil {
		return nil, errors.Errorf("placeholder %q: %w", placeholder.Name(), err)
	}

	placeholders = append(placeholders, placeholder)

	phs, err := Parse(str)
	if err != nil {
		return nil, err
	}

	placeholders = append(placeholders, phs...)

	return placeholders, nil
}

func (phs Placeholders) parsePlaceholder(str string) (Placeholder, string) { //nolint:ireturn
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

	if placeholder != nil || len(str) == 0 {
		return placeholder, str[optIndex:]
	}

	switch {
	case strings.HasPrefix(str, options.OptStartSign):
		// Unnamed placeholder, e.g. `%(content='...')`.
		return PlainText(""), str
	case strings.HasPrefix(str, "t"):
		// Placeholder indent, e.g. `%t`.
		return PlainText("\t"), str[1:]
	case strings.HasPrefix(str, "n"):
		// Placeholder newline, e.g. `%n`.
		return PlainText("\n"), str[1:]
	}

	return nil, str
}

func isPlaceholderNameCharacter(c byte) bool {
	// Check if the byte value falls within the range of alphanumeric characters
	return c == '-' || c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9')
}
