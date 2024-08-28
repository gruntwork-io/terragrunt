// Package formatter provides a logrus formatter that formats log entries in a structured way.
package formatter

import (
	"bytes"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/gruntwork-io/go-commons/errors"
	"github.com/sirupsen/logrus"
	"golang.org/x/exp/maps"
)

const (
	defaultTimestampForFormattedLayout = "15:04:05.000"
	defaultTimestamp                   = time.RFC3339

	PrefixKeyName   = "prefix"
	TFBinaryKeyName = "tfBinary"
	NoneLevel       = logrus.Level(10)
)

// Formatter implements logrus.Formatter
var _ logrus.Formatter = new(Formatter)

type PrefixStyle interface {
	// ColorFunc creates a closure to avoid computation ANSI color code.
	ColorFunc(prefixName string) ColorFunc
}

type Formatter struct {
	// Disable formatted layout
	DisableLogFormatting bool

	// Force disabling colors. For a TTY colors are enabled by default.
	DisableColors bool

	// Disable the conversion of the log levels to uppercase
	DisableUppercase bool

	// Timestamp format to use for display when a full timestamp is printed.
	TimestampFormat string

	// The fields are sorted by default for a consistent output.
	DisableSorting bool

	// Wrap empty fields in quotes if true.
	QuoteEmptyFields bool

	// Can be set to the override the default quoting character " with something else. For example: ', or `.
	QuoteCharacter string

	// PrefixStyle is used to assign different styles (colors) to each prefix.
	PrefixStyle PrefixStyle

	// Color scheme to use.
	colorScheme compiledColorScheme
}

// NewFormatter returns a new Formatter instance with default values.
func NewFormatter() *Formatter {
	return &Formatter{
		colorScheme: defaultColorScheme.Compile(),
		PrefixStyle: NewPrefixStyle(),
	}
}

func (formatter *Formatter) SetColorScheme(colorScheme *ColorScheme) {
	maps.Copy(formatter.colorScheme, colorScheme.Compile())
}

// Format implements logrus.Formatter
func (formatter *Formatter) Format(entry *logrus.Entry) ([]byte, error) {
	buf := entry.Buffer
	if buf == nil {
		buf = new(bytes.Buffer)
	}

	if !formatter.DisableLogFormatting {
		if err := formatter.printFormatted(buf, entry); err != nil {
			return nil, err
		}
	} else {
		if err := formatter.printKeyValue(buf, entry); err != nil {
			return nil, err
		}
	}

	if err := buf.WriteByte('\n'); err != nil {
		return nil, errors.WithStackTrace(err)
	}

	return buf.Bytes(), nil
}

func (formatter *Formatter) printKeyValue(buf *bytes.Buffer, entry *logrus.Entry) error {
	timestampFormat := formatter.TimestampFormat
	if timestampFormat == "" {
		timestampFormat = defaultTimestamp
	}

	if err := formatter.appendKeyValue(buf, "time", entry.Time.Format(timestampFormat), false); err != nil {
		return err
	}

	if err := formatter.appendKeyValue(buf, "level", formatter.levelText(entry.Level), true); err != nil {
		return err
	}

	if val, ok := entry.Data[PrefixKeyName]; ok && val != nil {
		if val := val.(string); val != "" {
			if err := formatter.appendKeyValue(buf, "prefix", val, true); err != nil {
				return err
			}
		}
	}

	if val, ok := entry.Data[TFBinaryKeyName]; ok && val != nil {
		if val := val.(string); val != "" {
			if err := formatter.appendKeyValue(buf, "binary", filepath.Base(val), true); err != nil {
				return err
			}
		}
	}

	if entry.Message != "" {
		if err := formatter.appendKeyValue(buf, "msg", entry.Message, true); err != nil {
			return err
		}
	}

	keys := formatter.keys(entry.Data, PrefixKeyName, TFBinaryKeyName)
	for _, key := range keys {
		if err := formatter.appendKeyValue(buf, key, entry.Data[key], true); err != nil {
			return err
		}
	}

	return nil
}

func (formatter *Formatter) printFormatted(buf *bytes.Buffer, entry *logrus.Entry) error {
	level := fmt.Sprintf("%-6s ", formatter.levelText(entry.Level))
	if !formatter.DisableUppercase {
		level = strings.ToUpper(level)
	}

	var (
		prefix    string
		tfBinary  string
		timestamp string
	)

	if val, ok := entry.Data[PrefixKeyName]; ok && val != nil {
		if val, ok := val.(string); ok && val != "" {
			prefix = fmt.Sprintf("[%s] ", val)
		}
	}

	if val, ok := entry.Data[TFBinaryKeyName]; ok && val != nil {
		if val, ok := val.(string); ok && val != "" {
			tfBinary = val + ": "
		}
	}

	timestampFormat := formatter.TimestampFormat
	if timestampFormat == "" {
		timestampFormat = defaultTimestampForFormattedLayout
	}

	timestamp = entry.Time.Format(timestampFormat) + " "

	if !formatter.DisableColors {
		level = formatter.colorScheme.LevelColorFunc(entry.Level)(level)
		prefix = formatter.PrefixStyle.ColorFunc(prefix)(prefix)
		tfBinary = formatter.colorScheme.ColorFunc(TFBinaryStyle)(tfBinary)
		timestamp = formatter.colorScheme.ColorFunc(TimestampStyle)(timestamp)
	}

	if _, err := fmt.Fprintf(buf, "%s%s%s%s%s", timestamp, level, prefix, tfBinary, entry.Message); err != nil {
		return errors.WithStackTrace(err)
	}

	keys := formatter.keys(entry.Data, PrefixKeyName, TFBinaryKeyName)
	for _, key := range keys {
		value := entry.Data[key]
		if err := formatter.appendKeyValue(buf, key, value, true); err != nil {
			return err
		}
	}

	return nil
}

func (formatter *Formatter) appendKeyValue(buf *bytes.Buffer, key string, value interface{}, appendSpace bool) error {
	keyFmt := "%s="
	if appendSpace {
		keyFmt = " " + keyFmt
	}

	if _, err := fmt.Fprintf(buf, keyFmt, key); err != nil {
		return errors.WithStackTrace(err)
	}

	if err := formatter.appendValue(buf, value); err != nil {
		return err
	}

	return nil
}

func (formatter *Formatter) appendValue(buf *bytes.Buffer, value interface{}) error {
	var str string

	switch value := value.(type) {
	case string:
		str = value
	case error:
		str = value.Error()
	default:
		if _, err := fmt.Fprint(buf, value); err != nil {
			return errors.WithStackTrace(err)
		}

		return nil
	}

	valueFmt := "%v"
	if formatter.needsQuoting(str) {
		valueFmt = formatter.QuoteCharacter + valueFmt + formatter.QuoteCharacter
	}

	if _, err := fmt.Fprintf(buf, valueFmt, value); err != nil {
		return errors.WithStackTrace(err)
	}

	return nil
}

func (formatter *Formatter) levelText(level logrus.Level) string {
	levelText := level.String()
	if level == logrus.WarnLevel {
		levelText = "warn"
	}

	if levelText == "unknown" {
		levelText = "stdout"
	}

	return levelText
}

func (formatter *Formatter) keys(data logrus.Fields, removeKeys ...string) []string {
	var (
		keys []string
	)

	for key := range data {
		var skip bool

		for _, removeKey := range removeKeys {
			if key == removeKey {
				skip = true
				break
			}
		}

		if !skip {
			keys = append(keys, key)
		}
	}

	if !formatter.DisableSorting {
		sort.Strings(keys)
	}

	return keys
}

func (formatter *Formatter) needsQuoting(text string) bool {
	if formatter.QuoteEmptyFields && len(text) == 0 {
		return true
	}

	for _, ch := range text {
		if !((ch >= 'a' && ch <= 'z') ||
			(ch >= 'A' && ch <= 'Z') ||
			(ch >= '0' && ch <= '9') ||
			ch == '-' || ch == '.') {
			return true
		}
	}

	return false
}
