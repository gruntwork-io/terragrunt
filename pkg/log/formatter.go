package log

import (
	"bytes"
	"encoding/json"
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
	defaultTimestampFormat             = time.RFC3339
	defaultOutputFormat                = KeyValueFormat

	PrefixKeyName   = "prefix"
	TFBinaryKeyName = "tfBinary"
)

// Formatter implements logrus.Formatter
var _ logrus.Formatter = new(Formatter)

type PrefixStyle interface {
	// ColorFunc creates a closure to avoid computation ANSI color code.
	ColorFunc(prefixName string) ColorFunc
}

type Formatter struct {
	// OutputFormat specifies the format in which the log will be displayed.
	OutputFormat Format

	// Disable the conversion of the log levels to uppercase
	DisableUppercase bool

	// DisableTimestamp allows disabling automatic timestamps in output
	DisableTimestamp bool

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

	// FieldMap allows users to customize the names of keys for default fields.
	// As an example:
	// formatter := &JSONFormatter{
	//   	FieldMap: FieldMap{
	// 		 FieldKeyTime:  "@timestamp",
	// 		 FieldKeyLevel: "@level",
	// 		 FieldKeyMsg:   "@message",
	// 		 FieldKeyFunc:  "@caller",
	//    },
	// }
	FieldMap FieldMap

	// Color scheme to use.
	colorScheme compiledColorScheme
}

// NewFormatter returns a new Formatter instance with default values.
func NewFormatter() *Formatter {
	return &Formatter{
		PrefixStyle:  NewPrefixStyle(),
		OutputFormat: defaultOutputFormat,
		colorScheme:  defaultColorScheme.Compile(),
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

	prefixFieldClashes(Fields(entry.Data), formatter.FieldMap)

	switch formatter.OutputFormat {
	case JSONFormat, JSONFormatIndent:
		indent := formatter.OutputFormat == JSONFormatIndent
		if err := formatter.printJSON(buf, entry, indent); err != nil {
			return nil, err
		}

	case KeyValueFormat:
		if err := formatter.printKeyValue(buf, entry); err != nil {
			return nil, err
		}

	case PrettyFormat, PrettyFormatNoColor:
		fallthrough
	default:
		noColor := formatter.OutputFormat == PrettyFormatNoColor
		if err := formatter.printPretty(buf, entry, noColor); err != nil {
			return nil, err
		}
	}

	return buf.Bytes(), nil
}

func (formatter *Formatter) printJSON(buf *bytes.Buffer, entry *logrus.Entry, indent bool) error {
	data := make(Fields, len(entry.Data)+3)

	for k, v := range entry.Data {
		switch v := v.(type) {
		case error:
			// Otherwise errors are ignored by `encoding/json` https://github.com/sirupsen/logrus/issues/137
			data[k] = v.Error()
		default:
			data[k] = v
		}
	}

	if !formatter.DisableTimestamp {
		timestampFormat := formatter.TimestampFormat
		if timestampFormat == "" {
			timestampFormat = defaultTimestampFormat
		}

		data[FieldKeyTime] = entry.Time.Format(timestampFormat)
	}
	data[FieldKeyMsg] = entry.Message
	data[FieldKeyLevel] = fromLogrusLevel(entry.Level).String()

	encoder := json.NewEncoder(buf)
	if indent {
		encoder.SetIndent("", "  ")
	}

	if err := encoder.Encode(data); err != nil {
		return errors.Errorf("failed to marshal fields to JSON, %w", err)
	}

	return nil
}

func (formatter *Formatter) printKeyValue(buf *bytes.Buffer, entry *logrus.Entry) error {
	if !formatter.DisableTimestamp {
		timestampFormat := formatter.TimestampFormat
		if timestampFormat == "" {
			timestampFormat = defaultTimestampFormat
		}

		if err := formatter.appendKeyValue(buf, "time", entry.Time.Format(timestampFormat), false); err != nil {
			return err
		}
	}

	if err := formatter.appendKeyValue(buf, "level", fromLogrusLevel(entry.Level), true); err != nil {
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

	if err := buf.WriteByte('\n'); err != nil {
		return errors.WithStackTrace(err)
	}

	return nil
}

func (formatter *Formatter) printPretty(buf *bytes.Buffer, entry *logrus.Entry, noColor bool) error {
	level := fmt.Sprintf("%-6s ", fromLogrusLevel(entry.Level))

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

	if !formatter.DisableTimestamp {
		timestampFormat := formatter.TimestampFormat
		if timestampFormat == "" {
			timestampFormat = defaultTimestampForFormattedLayout
		}

		timestamp = entry.Time.Format(timestampFormat) + " "
	}

	if !noColor {
		level = formatter.colorScheme.LevelColorFunc(fromLogrusLevel(entry.Level))(level)
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

	if err := buf.WriteByte('\n'); err != nil {
		return errors.WithStackTrace(err)
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
