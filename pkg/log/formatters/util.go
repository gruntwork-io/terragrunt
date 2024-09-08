package formatters

import (
	"strings"

	"github.com/gruntwork-io/go-commons/errors"
	"github.com/gruntwork-io/terragrunt/pkg/log"
)

const tagName = "opt"

func AllFormatters() log.Formatters {
	return []log.Formatter{
		NewPrettyFormatter(),
		NewKeyValueFormatter(),
		NewJSONFormatter(),
	}
}

// ParseFormat takes a string and returns a Formatter instance with defined options.
func ParseFormat(str string) (log.Formatter, error) {
	var (
		formatter     log.Formatter = NewPrettyFormatter()
		allFormatters               = AllFormatters()
		opts                        = make(map[string]any)
	)

	formatters := make(map[string]log.Formatter, len(allFormatters))
	for _, f := range allFormatters {
		formatters[f.Name()] = f
	}

	parts := strings.Split(str, ",")
	for _, name := range parts {
		name = strings.TrimSpace(name)
		name = strings.ToLower(name)

		if f, ok := formatters[name]; ok {
			formatter = f
			continue
		}

		var value any = true

		if parts := strings.SplitN(name, ":", 2); len(parts) > 1 {
			name = parts[0]
			value = parts[1]
		}

		if strings.HasPrefix(name, "no-") {
			name = name[3:]
			value = false
		}

		opts[name] = value
	}

	if formatter == nil {
		return nil, errors.Errorf("invalid format, supported formats: %s", strings.Join(formatter.SupportedOption(), ", "))
	}

	for name, value := range opts {
		if err := formatter.SetOption(name, value); err != nil {
			return nil, err
		}
	}

	return formatter, nil
}
