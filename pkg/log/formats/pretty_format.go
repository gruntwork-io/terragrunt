package formats

import (
	"bytes"
	"fmt"

	"github.com/gruntwork-io/go-commons/errors"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/sirupsen/logrus"
	"golang.org/x/exp/maps"
)

const (
	PrettyFormatName = "pretty"
)

// PrettyFormat implements formats.Format
var _ Format = new(PrettyFormat)

type PrettyFormat struct {
	*CommonFormat

	// PrefixStyle is used to assign different styles (colors) to each prefix.
	PrefixStyle PrefixStyle

	// Color scheme to use.
	colorScheme compiledColorScheme

	// Reuse for printing fields in key-value format
	keyValueFormatter *KeyValueFormat
}

// NewPrettyFormat returns a new PrettyFormat instance with default values.
func NewPrettyFormat(presets ...*Preset) *PrettyFormat {
	return &PrettyFormat{
		CommonFormat:      NewCommonFormat(PrettyFormatName, presets),
		PrefixStyle:       NewPrefixStyle(),
		colorScheme:       defaultColorScheme.Compile(),
		keyValueFormatter: &KeyValueFormat{},
	}
}

func (format *PrettyFormat) SetColorScheme(colorScheme *ColorScheme) {
	maps.Copy(format.colorScheme, colorScheme.Compile())
}

// Format implements logrus.Formatter
func (format *PrettyFormat) Format(entry *logrus.Entry) ([]byte, error) {
	buf := entry.Buffer
	if buf == nil {
		buf = new(bytes.Buffer)
	}

	var (
		level     = log.FromLogrusLevel(entry.Level)
		fields    = log.Fields(entry.Data)
		levelStr  string
		prefix    string
		subPrefix string
		timestamp string
	)

	if timestamp, _ = format.formatOption(OptionTime, entry.Time, level, entry.Message, log.Fields(entry.Data)); timestamp != "" {
		timestamp += " "
	}

	if levelStr, _ = format.formatOption(OptionLevel, entry.Time, level, entry.Message, log.Fields(entry.Data)); levelStr != "" {
		levelStr += " "
	}

	if prefix, _ = format.formatOption(OptionPrefix, entry.Time, level, entry.Message, log.Fields(entry.Data)); prefix != "" {
		prefix += " "
	}

	msg, _ := format.formatOption(OptionMsg, entry.Time, level, entry.Message, log.Fields(entry.Data))

	if opt := format.getOption(OptionColor, level); opt.enable {
		levelStr = format.colorScheme.LevelColorFunc(log.FromLogrusLevel(entry.Level))(levelStr)
		prefix = format.PrefixStyle.ColorFunc(prefix)(prefix)
		subPrefix = format.colorScheme.ColorFunc(SubPrefixStyle)(subPrefix)
		timestamp = format.colorScheme.ColorFunc(TimestampStyle)(timestamp)
	} else {
		msg = ansiReg.ReplaceAllString(msg, "")
	}

	if _, err := fmt.Fprintf(buf, "%s%s%s%s%s", timestamp, levelStr, prefix, subPrefix, msg); err != nil {
		return nil, errors.WithStackTrace(err)
	}

	keys := fields.Keys(log.FieldKeyPrefix, log.FieldKeySubPrefix)
	for _, key := range keys {
		value := fields[key]
		if val, ok := format.formatOption("fields."+key, entry.Time, level, entry.Message, log.Fields(entry.Data)); !ok {
			continue
		} else if val != "" {
			value = val
		}

		if err := format.keyValueFormatter.appendKeyValue(buf, key, value, true); err != nil {
			return nil, err
		}
	}

	if err := buf.WriteByte('\n'); err != nil {
		return nil, errors.WithStackTrace(err)
	}

	return buf.Bytes(), nil
}
