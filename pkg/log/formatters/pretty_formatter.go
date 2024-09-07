package formatters

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/gruntwork-io/go-commons/errors"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/sirupsen/logrus"
	"golang.org/x/exp/maps"
)

const (
	PrettyFormatterName = "pretty"

	defaultPrettyFormatterTimestampFormat = "15:04:05.000"
)

// PrettyFormatter implements log.Formatter
var _ log.Formatter = new(PrettyFormatter)

type PrettyFormatter struct {
	// Disable the conversion of the log levels to uppercase
	DisableUppercase bool

	// DisableTimestamp allows disabling automatic timestamps in output
	DisableTimestamp bool `opt:"no-timestamp"`

	// Timestamp format to use for display when a full timestamp is printed.
	TimestampFormat string

	// Force disabling colors. For a TTY colors are enabled by default.
	DisableColors bool `opt:"no-color"`

	// PrefixStyle is used to assign different styles (colors) to each prefix.
	PrefixStyle PrefixStyle

	// Color scheme to use.
	colorScheme compiledColorScheme

	// Reuse for printing fields in key-value format
	keyValueFormatter *KeyValueFormatter
}

// NewPrettyFormatter returns a new PrettyFormatter instance with default values.
func NewPrettyFormatter() *PrettyFormatter {
	return &PrettyFormatter{
		TimestampFormat:   defaultPrettyFormatterTimestampFormat,
		PrefixStyle:       NewPrefixStyle(),
		colorScheme:       defaultColorScheme.Compile(),
		keyValueFormatter: &KeyValueFormatter{},
	}
}

func (formatter *PrettyFormatter) SetColorScheme(colorScheme *ColorScheme) {
	maps.Copy(formatter.colorScheme, colorScheme.Compile())
}

// Name implements Formatter
func (formatter *PrettyFormatter) Name() string {
	return PrettyFormatterName
}

// Format implements logrus.Formatter
func (formatter *PrettyFormatter) Format(entry *logrus.Entry) ([]byte, error) {
	buf := entry.Buffer
	if buf == nil {
		buf = new(bytes.Buffer)
	}

	level := fmt.Sprintf("%-6s ", log.FromLogrusLevel(entry.Level))

	if !formatter.DisableUppercase {
		level = strings.ToUpper(level)
	}

	if !formatter.DisableUppercase {
		level = strings.ToUpper(level)
	}

	var (
		prefix    string
		tfBinary  string
		timestamp string
		fields    = log.Fields(entry.Data)
	)

	if val, ok := fields[log.FieldKeyPrefix]; ok && val != nil {
		if val, ok := val.(string); ok && val != "" {
			prefix = fmt.Sprintf("[%s] ", val)
		}
	}

	if val, ok := fields[log.FieldKeyTFBinary]; ok && val != nil {
		if val, ok := val.(string); ok && val != "" {
			tfBinary = val + ": "
		}
	}

	if !formatter.DisableTimestamp && formatter.TimestampFormat != "" {
		timestamp = entry.Time.Format(formatter.TimestampFormat) + " "
	}

	if !formatter.DisableColors {
		level = formatter.colorScheme.LevelColorFunc(log.FromLogrusLevel(entry.Level))(level)
		prefix = formatter.PrefixStyle.ColorFunc(prefix)(prefix)
		tfBinary = formatter.colorScheme.ColorFunc(TFBinaryStyle)(tfBinary)
		timestamp = formatter.colorScheme.ColorFunc(TimestampStyle)(timestamp)
	}

	if _, err := fmt.Fprintf(buf, "%s%s%s%s%s", timestamp, level, prefix, tfBinary, entry.Message); err != nil {
		return nil, errors.WithStackTrace(err)
	}

	keys := fields.Keys(log.FieldKeyPrefix, log.FieldKeyTFBinary)
	for _, key := range keys {
		value := fields[key]
		if err := formatter.keyValueFormatter.appendKeyValue(buf, key, value, true); err != nil {
			return nil, err
		}
	}

	if err := buf.WriteByte('\n'); err != nil {
		return nil, errors.WithStackTrace(err)
	}

	return buf.Bytes(), nil
}
