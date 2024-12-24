// Package placeholders represents a set of placeholders for formatting various log values.
package placeholders

import (
	"strings"

	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/pkg/log/format/options"
)

const placeholderSign = '%'

// Placeholder is part of the log message, used to format different log values.
type Placeholder interface {
	// Name returns a placeholder name.
	Name() string
	// GetOption returns the option with the given option name.
	GetOption(name string) (options.Option, error)
	// Format returns the formatted value.
	Format(data *options.Data) (string, error)
}

// Placeholders are a set of Placeholders.
type Placeholders []Placeholder

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
		registered   = newPlaceholders()
		placeholders Placeholders
		next         int
	)

	for index := 0; index < len(str); index++ {
		char := str[index]

		if char == placeholderSign {
			if index+1 >= len(str) {
				return nil, errors.Errorf("empty placeholder name")
			}

			if str[index+1] == placeholderSign {
				str = str[:index] + str[index+1:]

				continue
			}

			if next != index {
				placeholder := PlainText(str[next:index])
				placeholders = append(placeholders, placeholder)
			}

			placeholder, num, err := parsePlaceholder(str[index+1:], registered)
			if err != nil {
				return nil, err
			}

			placeholders = append(placeholders, placeholder)
			index += num + 1
			next = index + 1
		}
	}

	if next != len(str) {
		placeholder := PlainText(str[next:])
		placeholders = append(placeholders, placeholder)
	}

	return placeholders, nil
}

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

func parsePlaceholderOption(placeholder Placeholder, str string) (int, error) {
	var (
		nextOptStart int
		quoted       byte
		option       options.Option
	)

	for index := range len(str) {
		char := str[index]

		if char == '"' || char == '\'' {
			if quoted == 0 {
				quoted = char
			} else if quoted == char && index > 0 && str[index-1] != '\\' {
				quoted = 0
			}
		}

		if quoted != 0 {
			continue
		}

		if char != '=' && char != ',' && char != ')' {
			continue
		}

		val := str[nextOptStart:index]
		val = strings.TrimSpace(val)
		val = strings.Trim(val, "'")
		val = strings.Trim(val, "\"")

		if nextOptStart > 0 && str[nextOptStart-1] == '=' {
			if option == nil {
				return 0, errors.Errorf("empty option name for placeholder %q", placeholder.Name())
			}

			if err := option.ParseValue(val); err != nil {
				return 0, errors.Errorf("invalid value %q for option %q, placeholder %q: %w", val, option.Name(), placeholder.Name(), err)
			}
		} else if val != "" {
			opt, err := placeholder.GetOption(val)
			if err != nil {
				return 0, errors.Errorf("invalid option name %q for placeholder %q: %w", val, placeholder.Name(), err)
			}

			option = opt
		}

		nextOptStart = index + 1

		if char == ')' {
			return index + 1, nil
		}
	}

	return 0, errors.Errorf("invalid option %q for placeholder %q", str[nextOptStart:], placeholder.Name())
}

func parsePlaceholder(str string, registered Placeholders) (Placeholder, int, error) {
	var (
		placeholder Placeholder
		optStart    int
	)

	for index := range len(str) {
		char := str[index]

		switch {
		case index == 0 && char == '(':
			placeholder = PlainText("")
		case isPlaceholderNameCharacter(char):
			name := str[:index+1]

			if pl := registered.Get(name); pl != nil {
				placeholder = pl
				optStart = index + 1
			}

			continue
		}

		if placeholder == nil {
			break
		}

		optStr := str[optStart:]

		if len(optStr) != 0 && optStr[0] == '(' {
			optLen, err := parsePlaceholderOption(placeholder, optStr[1:])

			return placeholder, index + optLen, err
		}

		return placeholder, optStart - 1, nil
	}

	if placeholder != nil {
		return placeholder, len(str) - 1, nil
	}

	switch str[0] {
	case 't':
		return PlainText("\t"), 0, nil
	case 'n':
		return PlainText("\n"), 0, nil
	}

	return nil, 0, errors.Errorf("invalid placeholder name %q, available values: %s", str, strings.Join(registered.Names(), ","))
}

func isPlaceholderNameCharacter(c byte) bool {
	// Check if the byte value falls within the range of alphanumeric characters
	return c == '-' || c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9')
}
