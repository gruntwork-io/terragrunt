package formats

import (
	"bytes"
	"encoding/json"
	"time"

	"github.com/gruntwork-io/go-commons/errors"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/sirupsen/logrus"
)

const (
	JSONFormatterName = "json"

	defaultJSONFormatterTimestampFormat = time.RFC3339
)

// JSONFormatter implements formats.Formatter
var _ Formatter = new(JSONFormatter)

type JSONFormatter struct {
	*CommonFormatter

	// DisableTimestamp allows disabling automatic timestamps in output
	DisableTimestamp bool

	// Timestamp format to use for display when a full timestamp is printed.
	TimestampFormat string

	// EnableIndent enables indent.
	EnableIndent bool
}

// NewJSONFormatter returns a new JSONFormatter instance with default values.
func NewJSONFormatter() *JSONFormatter {
	return &JSONFormatter{
		CommonFormatter: &CommonFormatter{
			name: JSONFormatterName,
		},
	}
}

// Format implements logrus.Formatter
func (formatter *JSONFormatter) Format(entry *logrus.Entry) ([]byte, error) {
	buf := entry.Buffer
	if buf == nil {
		buf = new(bytes.Buffer)
	}

	fields := make(log.Fields, len(entry.Data)+3)

	for k, v := range entry.Data {
		switch v := v.(type) {
		case error:
			// Otherwise errors are ignored by `encoding/json`
			fields[k] = v.Error()
		default:
			fields[k] = v
		}
	}

	if !formatter.DisableTimestamp && formatter.TimestampFormat != "" {
		fields[log.FieldKeyTime] = entry.Time.Format(formatter.TimestampFormat)
	}
	fields[log.FieldKeyMsg] = entry.Message
	fields[log.FieldKeyLevel] = log.FromLogrusLevel(entry.Level).String()

	encoder := json.NewEncoder(buf)
	if formatter.EnableIndent {
		encoder.SetIndent("", "  ")
	}

	if err := encoder.Encode(fields); err != nil {
		return nil, errors.Errorf("failed to marshal fields to JSON, %w", err)
	}

	return buf.Bytes(), nil
}
