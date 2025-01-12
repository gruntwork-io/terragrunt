package format

import (
	"bytes"
	"sync"

	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/pkg/log/format/options"
	"github.com/gruntwork-io/terragrunt/pkg/log/format/placeholders"
)

// Formatter implements logrus.Formatter.
var _ log.Formatter = new(Formatter)

type Formatter struct {
	baseDir        string
	placeholders   placeholders.Placeholders
	DisableColors  bool
	DisableOutput  bool
	relativePather *options.RelativePather
	mu             sync.Mutex
}

// NewFormatter returns a new Formatter instance with default values.
func NewFormatter(phs placeholders.Placeholders) *Formatter {
	return &Formatter{
		placeholders: phs,
	}
}

// Format implements logrus.Format.
func (formatter *Formatter) Format(entry *log.Entry) ([]byte, error) {
	if formatter.placeholders == nil || formatter.DisableOutput {
		return nil, nil
	}

	buf := entry.Buffer
	if buf == nil {
		buf = new(bytes.Buffer)
	}

	str, err := formatter.placeholders.Format(&options.Data{
		Entry:          entry,
		BaseDir:        formatter.baseDir,
		DisableColors:  formatter.DisableColors,
		RelativePather: formatter.relativePather,
	})
	if err != nil {
		return nil, err
	}

	formatter.mu.Lock()
	defer formatter.mu.Unlock()

	if str != "" {
		if _, err := buf.WriteString(str); err != nil {
			return nil, errors.New(err)
		}

		if err := buf.WriteByte('\n'); err != nil {
			return nil, errors.New(err)
		}
	}

	return buf.Bytes(), nil
}

func (formatter *Formatter) SetBaseDir(baseDir string) error {
	pather, err := options.NewRelativePather(baseDir)
	if err != nil {
		return err
	}

	formatter.relativePather = pather
	formatter.baseDir = baseDir

	return nil
}

// DisableRelativePaths disables the conversion of absolute paths to relative ones.
func (formatter *Formatter) DisableRelativePaths() {
	formatter.relativePather = nil
}

// SetPlaceholders sets log placeholders.
func (formatter *Formatter) SetPlaceholders(phs placeholders.Placeholders) {
	formatter.placeholders = phs
}

// SetFormat parses and sets log format.
func (formatter *Formatter) SetFormat(str string) error {
	phs, err := ParseFormat(str)
	if err != nil {
		return err
	}

	formatter.placeholders = phs

	return nil
}

// SetCustomFormat parses and sets custom log format.
func (formatter *Formatter) SetCustomFormat(str string) error {
	phs, err := placeholders.Parse(str)
	if err != nil {
		return err
	}

	formatter.placeholders = phs

	return nil
}
