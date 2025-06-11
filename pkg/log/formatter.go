package log

import (
	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/sirupsen/logrus"
)

// Formatter is used to implement a custom Formatter.
type Formatter interface {
	// SetDisabledColors enables/disables log colors.
	SetDisabledColors(val bool)
	// DisabledColors returns true if log colors are disabled.
	DisabledColors() bool
	// SetDisabledOutput  enables/disables log output.
	SetDisabledOutput(val bool)
	// DisabledOutput returns true if log output is disabled.
	DisabledOutput() bool
	// SetBaseDir creates a set of relative paths that are used to convert full paths to relative ones.
	SetBaseDir(baseDir string) error
	// DisableRelativePaths disables the conversion of absolute paths to relative ones.
	DisableRelativePaths()
	// SetFormat parses and sets log format.
	SetFormat(str string) error
	// SetCustomFormat parses and sets custom log format.
	SetCustomFormat(str string) error

	// Format takes an `Entry`. It exposes all the fields, including the default ones:
	//
	// * `entry.Data["msg"]`. The message passed from Info, Warn, Error ..
	// * `entry.Data["time"]`. The timestamp.
	// * `entry.Data["level"]. The level the entry was logged at.
	//
	// Any additional fields added with `WithField` or `WithFields` are also in
	// `entry.Data`. Format is expected to return an array of bytes which are then
	// logged to `logger.Out`.
	Format(entry *Entry) ([]byte, error)
}

// Entry is the final logging entry.
type Entry struct {
	*logrus.Entry
	Fields Fields
	Level  Level
}

// fromLogrusFormatter converts call from logrus.Formatter interface to our long.Formatter interface.
type fromLogrusFormatter struct {
	Formatter
}

func (f *fromLogrusFormatter) Format(parent *logrus.Entry) ([]byte, error) {
	if parent == nil {
		return nil, errors.Errorf("nil entry provided")
	}

	entry := &Entry{
		Entry:  parent,
		Level:  FromLogrusLevel(parent.Level),
		Fields: Fields(parent.Data),
	}

	return f.Formatter.Format(entry)
}
