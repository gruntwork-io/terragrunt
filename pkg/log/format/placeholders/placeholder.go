// Package placeholders implements fillers from which to format logs.
package placeholders

import (
	"strings"

	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/pkg/log/format/options"
)

const placeholderSign = '%'

type Placeholders []Placeholder

func (phs Placeholders) Get(name string) Placeholder {
	for _, ph := range phs {
		if ph.Name() == name {
			return ph
		}
	}

	return nil
}

func newPlaceholders() Placeholders {
	return Placeholders{
		Interval(),
		Time(),
		Level(),
		Message(),
		Field(WorkDirKeyName, options.PathFormat(options.NonePath, options.RelativePath, options.RelativeModulePath, options.ModulePath)),
		Field(TFPathKeyName, options.PathFormat(options.NonePath, options.FilenamePath, options.DirectoryPath)),
	}
}

func parsePlaceholder(str string, registered Placeholders) (Placeholder, int, error) {
	var (
		next        int
		quoted      byte
		placeholder Placeholder
		option      options.Option
	)

	for i := 0; i < len(str); i++ {
		ch := str[i]

		if ch == '"' || ch == '\'' {
			if quoted == 0 {
				quoted = ch
			} else if i > 0 && str[i-1] != '\\' {
				quoted = 0
			}
		}

		if quoted != 0 {
			continue
		}

		if placeholder == nil {
			if !isPlaceholderName(ch) {
				return nil, 0, errors.Errorf("invalid placeholder name %q", str[next:i])
			}

			name := str[next : i+1]

			if placeholder = registered.Get(name); placeholder != nil {
				next = i + 2 //nolint:mnd
			}

			continue
		}

		if next-1 == i && ch != '(' {
			return placeholder, i - 1, nil
		}

		if ch == '=' || ch == ',' || ch == ')' {
			val := str[next:i]
			val = strings.Trim(val, "'")
			val = strings.Trim(val, "\"")

			if str[next-1] == '=' {
				if option == nil {
					return nil, 0, errors.Errorf("empty option name for placeholder %q", placeholder.Name())
				}

				if err := option.ParseValue(val); err != nil {
					return nil, 0, errors.Errorf("invalid value %q for option %q, placeholder %q: %w", val, option.Name(), placeholder.Name(), err)
				}
			} else if val != "" {
				if option = placeholder.GetOption(val); option == nil {
					return nil, 0, errors.Errorf("invalid option name %q for placeholder %q", val, placeholder.Name())
				}
			}

			next = i + 1
		}

		if ch == ')' {
			return placeholder, i, nil
		}
	}

	if placeholder == nil {
		return nil, 0, errors.Errorf("invalid placeholder name %q", str)
	}

	if next < len(str) {
		return nil, 0, errors.Errorf("invalid option %q for placeholder %q", str[next:], placeholder.Name())
	}

	return placeholder, len(str) - 1, nil
}

func Parse(str string) (Placeholders, error) {
	var (
		registered   = newPlaceholders()
		placeholders Placeholders
		next         int
	)

	for i := 0; i < len(str); i++ {
		ch := str[i]

		if ch == placeholderSign {
			if i+1 >= len(str) {
				return nil, errors.Errorf("empty placeholder name")
			}

			if str[i+1] == placeholderSign {
				str = str[:i] + str[i+1:]
				continue
			}

			if next != i {
				placeholder := PlainText(str[next:i])
				placeholders = append(placeholders, placeholder)
			}

			placeholder, num, err := parsePlaceholder(str[i+1:], registered)
			if err != nil {
				return nil, err
			}

			placeholders = append(placeholders, placeholder)
			i += num + 1
			next = i + 1
		}
	}

	return placeholders, nil
}

func (phs Placeholders) Evaluate(data *options.Data) (string, error) {
	var str string

	for _, ph := range phs {
		s, err := ph.Evaluate(data)
		if err != nil {
			return "", err
		}

		str += s
	}

	return str, nil
}

type Placeholder interface {
	Name() string
	GetOption(name string) options.Option
	Evaluate(data *options.Data) (string, error)
}

func isPlaceholderName(c byte) bool {
	// Check if the byte value falls within the range of alphanumeric characters
	return c == '-' || c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9')
}
