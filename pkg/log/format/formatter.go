package format

import (
	"bytes"

	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/pkg/log/format/options"
	"github.com/gruntwork-io/terragrunt/pkg/log/format/placeholders"
)

// Formatter implements logrus.Formatter
var _ log.Formatter = new(Formatter)

type Formatter struct {
	format         placeholders.Placeholders
	disableColors  bool
	relativePather *options.RelativePather
}

// NewFormatter returns a new Formatter instance with default values.
func NewFormatter() *Formatter {
	return &Formatter{
		format: presets[PrettyFormat],
	}
}

// Format implements logrus.Format
func (formatter *Formatter) Format(entry *log.Entry) ([]byte, error) {
	if formatter.format == nil {
		return nil, nil
	}

	buf := entry.Buffer
	if buf == nil {
		buf = new(bytes.Buffer)
	}

	str := formatter.format.Evaluate(&options.Data{
		Entry:          entry,
		DisableColors:  formatter.disableColors,
		RelativePather: formatter.relativePather,
	})

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

// DisableColors disables log color
func (formatter *Formatter) DisableColors() {
	formatter.disableColors = true
}

func (formatter *Formatter) CreateRelativePathsCache(baseDir string) error {
	pather, err := options.NewRelativePather(baseDir)
	if err != nil {
		return err
	}

	formatter.relativePather = pather

	return nil
}

func (formatter *Formatter) SetFormat(format placeholders.Placeholders) {
	formatter.format = format
}
