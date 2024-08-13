package formatter

import (
	"bytes"
	"fmt"
	"runtime"
	"sort"
	"strings"

	"github.com/gruntwork-io/go-commons/errors"
	"github.com/sirupsen/logrus"
)

const (
	PrefixKeyName = "prefix"
	NoneLevel     = logrus.Level(10)
)

type Formatter struct {
	// Set to true to bypass checking for a TTY before outputting colors.
	ForceColors bool

	// Force disabling colors. For a TTY colors are enabled by default.
	DisableColors bool

	// Disable the conversion of the log levels to uppercase
	DisableUppercase bool

	// Enable logging the full timestamp when a TTY is attached instead of just the time passed since beginning of execution.
	FullTimestamp bool

	// Force formatted layout, even for non-TTY output.
	ForceFormatting bool

	// Timestamp format to use for display when a full timestamp is printed.
	TimestampFormat string

	// The fields are sorted by default for a consistent output.
	DisableSorting bool

	// Wrap empty fields in quotes if true.
	QuoteEmptyFields bool

	// Can be set to the override the default quoting character " with something else. For example: ', or `.
	QuoteCharacter string

	// Color scheme to use.
	colorScheme compiledColorScheme
}

// NewFormatter returns a new Formatter instance with default values.
func NewFormatter(disableColors bool, timestampFormat string) logrus.Formatter {
	return &Formatter{
		FullTimestamp:   true,
		DisableColors:   disableColors,
		TimestampFormat: timestampFormat,
		colorScheme:     defaultColorScheme.Complite(),
	}
}

func (formatter *Formatter) SetColorScheme(colorScheme *ColorScheme) {
	formatter.colorScheme = colorScheme.Complite()
}

func (formatter *Formatter) Format(entry *logrus.Entry) ([]byte, error) {
	buf := entry.Buffer
	if buf == nil {
		buf = new(bytes.Buffer)
	}

	if formatter.ForceFormatting || isTerminal {
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
	if err := formatter.appendKeyValue(buf, "time", entry.Time.Format(formatter.TimestampFormat), false); err != nil {
		return err
	}

	if err := formatter.appendKeyValue(buf, "level", entry.Level.String(), true); err != nil {
		return err
	}

	if entry.Message != "" {
		if err := formatter.appendKeyValue(buf, "msg", entry.Message, true); err != nil {
			return err
		}
	}

	for _, key := range formatter.keys(entry.Data) {
		if err := formatter.appendKeyValue(buf, key, entry.Data[key], true); err != nil {
			return err
		}
	}

	return nil
}

func (formatter *Formatter) printFormatted(buf *bytes.Buffer, entry *logrus.Entry) error {
	level := fmt.Sprintf("%5s ", formatter.levelText(entry.Level))

	prefix := ""
	if val, ok := entry.Data[PrefixKeyName]; ok {
		prefix = fmt.Sprintf("[%s]  ", val)
	}

	var timestamp string
	if formatter.FullTimestamp {
		timestamp = entry.Time.Format(formatter.TimestampFormat)
	} else {
		timestamp = fmt.Sprintf("%04d", miniTS())
	}

	if formatter.isColored() {
		level = formatter.colorScheme.LevelColorFunc(entry.Level)(level)
		prefix = formatter.colorScheme.ColorFunc(PrefixStyle)(prefix)
		timestamp = formatter.colorScheme.ColorFunc(TimestampStyle)(timestamp)
	}

	if _, err := fmt.Fprintf(buf, "%s %s%s%s", timestamp, level, prefix, entry.Message); err != nil {
		return errors.WithStackTrace(err)
	}

	for i, key := range formatter.keys(entry.Data, PrefixKeyName) {
		value := entry.Data[key]
		if err := formatter.appendKeyValue(buf, key, value, i != 0); err != nil {
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
		levelText = ""
	}

	if !formatter.DisableUppercase {
		return strings.ToUpper(levelText)
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

func (formatter *Formatter) isColored() bool {
	isColored := formatter.ForceColors || (isTerminal && (runtime.GOOS != "windows"))
	return isColored && !formatter.DisableColors
}
