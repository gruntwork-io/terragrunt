package formatters

import (
	"fmt"
	"strings"
	"time"

	"github.com/gruntwork-io/go-commons/collections"
	"github.com/gruntwork-io/go-commons/errors"
	"github.com/gruntwork-io/terragrunt/pkg/log"
)

const (
	optNameTime   = "time"
	optNameLevel  = "level"
	optNamePrefix = "prefix"
)

var (
	commonSupportedOptions = []string{"time", "level", "prefix", "tf-time", "tf-level", "tf-prefix"}

	timestampFormatMap = map[string]string{
		"%Y":           "2006",
		"%y":           "06",
		"%m":           "01",
		"%n":           "1",
		"%M":           "Jan",
		"%j":           "2",
		"%d":           "02",
		"%D":           "Mon",
		"%l":           "Monday",
		"%A":           "PM",
		"%a":           "pm",
		"%H":           "15",
		"%h":           "03",
		"%g":           "3",
		"%i":           "04",
		"%s":           "05",
		"%u":           ".000000",
		"%v":           ".000",
		"%T":           "MST",
		"%O":           "-0700",
		"%P":           "-07:00",
		"rfc3339":      time.RFC3339,
		"rfc3339-nano": time.RFC3339Nano,
		"date-time":    time.DateTime,
		"date-only":    time.DateOnly,
		"time-only":    time.TimeOnly,
	}

	levelShorts = map[log.Level]string{
		log.StderrLevel: "S",
		log.StdoutLevel: "S",
		log.ErrorLevel:  "E",
		log.WarnLevel:   "W",
		log.InfoLevel:   "I",
		log.DebugLevel:  "D",
		log.TraceLevel:  "T",
	}
)

type CommonFormatter struct {
	// Timestamp format to use for display when a full timestamp is printed.
	TimestampFormat string

	name             string
	options          map[string]any
	supportedOptions []string
	baseTimestamp    time.Time
}

func (formatter *CommonFormatter) Name() string {
	return formatter.name
}

func (formatter *CommonFormatter) SupportedOption() []string {
	return formatter.supportedOptions
}

func (formatter *CommonFormatter) SetOption(name string, value any) error {
	if !collections.ListContainsElement(formatter.supportedOptions, name) {
		return errors.Errorf("invalid option %q for the format %q, supprted options: %s", name, formatter.Name(), strings.Join(formatter.supportedOptions, ", "))
	}

	formatter.options[name] = value
	return nil
}

func (formatter *CommonFormatter) getTimestamp(t time.Time) string {
	var timestampFormat = formatter.TimestampFormat

	if val, ok := formatter.options[optNameTime]; ok {
		switch val := val.(type) {
		case bool:
			if val == false {
				return ""
			}

		case string:
			for old, new := range timestampFormatMap {
				val = strings.ReplaceAll(val, old, new)
			}

			val = t.Format(val)
			val = strings.ReplaceAll(val, "mini", fmt.Sprintf("%04d", time.Since(formatter.baseTimestamp)/time.Second))

			return val
		}
	}

	return t.Format(timestampFormat)
}

func (formatter *CommonFormatter) getLevel(level log.Level) string {
	var levelStr = fmt.Sprintf("%-6s", strings.ToUpper(level.String()))

	if val, ok := formatter.options[optNameLevel]; ok {
		switch val := val.(type) {
		case bool:
			if val == false {
				return ""
			}

		case string:
			switch strings.ToLower(val) {
			case "short":
				if short, ok := levelShorts[level]; ok {
					levelStr = short
				}
			}
		}
	}

	return levelStr
}

func (formatter *CommonFormatter) getPrefix(prefix any) string {
	var absPath, relPath string

	switch prefix := prefix.(type) {
	case string:
		relPath = prefix
		absPath = prefix
	case func() (string, string):
		absPath, relPath = prefix()
	}

	if val, ok := formatter.options[optNamePrefix]; ok {
		switch val := val.(type) {
		case bool:
			if val == false {
				return ""
			}

		case string:
			switch strings.ToLower(val) {
			case "abs-path":
				return absPath

			case "rel-path":
				return relPath
			}
		}
	}

	return relPath
}
