package formatters

import (
	"bytes"
	"fmt"
	"strings"
	"time"

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
	*CommonFormatter

	// Force disabling colors. For a TTY colors are enabled by default.
	DisableColors bool

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
		CommonFormatter: &CommonFormatter{
			TimestampFormat:  defaultPrettyFormatterTimestampFormat,
			name:             PrettyFormatterName,
			options:          make(map[string]any),
			supportedOptions: commonSupportedOptions,
			baseTimestamp:    time.Now(),
		},
		PrefixStyle:       NewPrefixStyle(),
		colorScheme:       defaultColorScheme.Compile(),
		keyValueFormatter: &KeyValueFormatter{},
	}
}

func (formatter *PrettyFormatter) SetColorScheme(colorScheme *ColorScheme) {
	maps.Copy(formatter.colorScheme, colorScheme.Compile())
}

// Format implements logrus.Formatter
func (formatter *PrettyFormatter) Format(entry *logrus.Entry) ([]byte, error) {
	buf := entry.Buffer
	if buf == nil {
		buf = new(bytes.Buffer)
	}

	var (
		level     string
		prefix    string
		tfBinary  string
		timestamp string
		fields    = log.Fields(entry.Data)
	)

	if level = formatter.getLevel(log.FromLogrusLevel(entry.Level)); level != "" {
		level += " "
	}

	if val, ok := fields[log.FieldKeyPrefix]; ok && val != nil {
		if prefix = formatter.getPrefix(val); prefix != "" {
			if strings.HasPrefix(prefix, log.CurDirWithSeparator) {
				prefix = prefix[len(log.CurDirWithSeparator):]
			}
			if prefix != "" {
				prefix = fmt.Sprintf("[%s] ", prefix)
			}
		}
	}

	if val, ok := fields[log.FieldKeyTFBinary]; ok && val != nil {
		if val, ok := val.(string); ok && val != "" {
			tfBinary = val + ": "
		}
	}

	if timestamp = formatter.getTimestamp(entry.Time); timestamp != "" {
		timestamp += " "
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
