package formats

import (
	"bytes"
	"fmt"
	"path/filepath"
	"time"

	"github.com/gruntwork-io/go-commons/errors"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/sirupsen/logrus"
)

const (
	KeyValueFormatterName = "key-value"

	defaultKeyValueFormatterTimestampFormat = time.RFC3339
)

// KeyValueFormatter implements formats.Formatter
var _ Formatter = new(KeyValueFormatter)

type KeyValueFormatter struct {
	*CommonFormatter

	// DisableTimestamp allows disabling automatic timestamps in output
	DisableTimestamp bool

	// Timestamp format to use for display when a full timestamp is printed.
	TimestampFormat string

	// Wrap empty fields in quotes if true.
	QuoteEmptyFields bool

	// Can be set to the override the default quoting character " with something else. For example: ', or `.
	QuoteCharacter string
}

// NewKeyValueFormatter returns a new KeyValueFormatter instance with default values.
func NewKeyValueFormatter() *KeyValueFormatter {
	return &KeyValueFormatter{
		CommonFormatter: &CommonFormatter{
			name: KeyValueFormatterName,
		},
	}
}

// Format implements logrus.Formatter
func (formatter *KeyValueFormatter) Format(entry *logrus.Entry) ([]byte, error) {
	buf := entry.Buffer
	if buf == nil {
		buf = new(bytes.Buffer)
	}

	var fields = log.Fields(entry.Data)

	if !formatter.DisableTimestamp && formatter.TimestampFormat != "" {
		if err := formatter.appendKeyValue(buf, log.FieldKeyTime, entry.Time.Format(formatter.TimestampFormat), false); err != nil {
			return nil, err
		}
	}

	if err := formatter.appendKeyValue(buf, log.FieldKeyLevel, log.FromLogrusLevel(entry.Level), true); err != nil {
		return nil, err
	}

	if val, ok := fields[log.FieldKeyPrefix]; ok && val != nil {
		if val := val.(string); val != "" {
			if err := formatter.appendKeyValue(buf, log.FieldKeyPrefix, val, true); err != nil {
				return nil, err
			}
		}
	}

	if val, ok := fields[log.FieldKeyCmd]; ok && val != nil {
		if val := val.(string); val != "" {
			if err := formatter.appendKeyValue(buf, log.FieldKeyCmd, filepath.Base(val), true); err != nil {
				return nil, err
			}
		}
	}

	if entry.Message != "" {
		if err := formatter.appendKeyValue(buf, log.FieldKeyMsg, entry.Message, true); err != nil {
			return nil, err
		}
	}

	keys := fields.Keys(log.FieldKeyPrefix, log.FieldKeyCmd)
	for _, key := range keys {
		if err := formatter.appendKeyValue(buf, key, fields[key], true); err != nil {
			return nil, err
		}
	}

	if err := buf.WriteByte('\n'); err != nil {
		return nil, errors.WithStackTrace(err)
	}

	return buf.Bytes(), nil
}

func (formatter *KeyValueFormatter) appendKeyValue(buf *bytes.Buffer, key string, value interface{}, appendSpace bool) error {
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

func (formatter *KeyValueFormatter) appendValue(buf *bytes.Buffer, value interface{}) error {
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

func (formatter *KeyValueFormatter) needsQuoting(text string) bool {
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
