package formats

import (
	"bytes"
	"fmt"
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

// PrettyFormatter implements formats.Formatter
var _ Formatter = new(PrettyFormatter)

type PrettyFormatter struct {
	*CommonFormatter

	// PrefixStyle is used to assign different styles (colors) to each prefix.
	PrefixStyle PrefixStyle

	// Color scheme to use.
	colorScheme compiledColorScheme

	// Reuse for printing fields in key-value format
	keyValueFormatter *KeyValueFormatter
}

// NewPrettyFormatter returns a new PrettyFormatter instance with default values.
func NewPrettyFormatter(presets ...*Preset) *PrettyFormatter {
	return &PrettyFormatter{
		CommonFormatter: &CommonFormatter{
			name:             PrettyFormatterName,
			supportedOptions: commonSupportedOptions,
			supportedPresets: presets,
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
		level     = log.FromLogrusLevel(entry.Level)
		fields    = log.Fields(entry.Data)
		levelStr  string
		prefix    string
		tfBinary  string
		timestamp string
	)

	if levelStr = formatter.getLevel(level); levelStr != "" {
		levelStr += " "
	}

	if val, ok := fields[log.FieldKeyPrefix]; ok && val != nil {
		if prefix = formatter.getPrefix(level, val); prefix != "" {
			prefix += " "
		}
	}

	if val, ok := fields[log.FieldKeyCmd]; ok && val != nil {
		if val, ok := val.(string); ok && val != "" {
			tfBinary = val + ": "
		}
	}

	if timestamp = formatter.getTimestamp(level, entry.Time); timestamp != "" {
		timestamp += " "
	}

	if opt := formatter.getOption(OptionColor, level); opt.enable {
		levelStr = formatter.colorScheme.LevelColorFunc(log.FromLogrusLevel(entry.Level))(levelStr)
		prefix = formatter.PrefixStyle.ColorFunc(prefix)(prefix)
		tfBinary = formatter.colorScheme.ColorFunc(TFBinaryStyle)(tfBinary)
		timestamp = formatter.colorScheme.ColorFunc(TimestampStyle)(timestamp)
	} else {
		entry.Message = ansiReg.ReplaceAllString(entry.Message, "")
	}

	if _, err := fmt.Fprintf(buf, "%s%s%s%s%s", timestamp, levelStr, prefix, tfBinary, entry.Message); err != nil {
		return nil, errors.WithStackTrace(err)
	}

	keys := fields.Keys(log.FieldKeyPrefix, log.FieldKeyCmd)
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
