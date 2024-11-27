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
	disableColors  bool
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
	if formatter.placeholders == nil {
		return nil, nil
	}

	buf := entry.Buffer
	if buf == nil {
		buf = new(bytes.Buffer)
	}

	str, err := formatter.placeholders.Format(&options.Data{
		Entry:          entry,
		BaseDir:        formatter.baseDir,
		DisableColors:  formatter.disableColors,
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

// DisableColors disables log color.
func (formatter *Formatter) DisableColors() {
	formatter.disableColors = true
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

// SetFormat sets log format.
func (formatter *Formatter) SetFormat(phs placeholders.Placeholders) {
	formatter.placeholders = phs
}
