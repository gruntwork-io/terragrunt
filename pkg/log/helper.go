package log

import (
	"github.com/sirupsen/logrus"
)

// Formatter is used to implement a custom Formatter.
type Formatter interface {
	Format(entry *Entry) ([]byte, error)
}

// Entry is the final logging entry.
type Entry struct {
	*logrus.Entry
	Level  Level
	Fields Fields
}

// fromLogrusFormatter converts call from logrus.Formatter interface to our long.Formatter interface.
type fromLogrusFormatter struct {
	Formatter
}

func (f *fromLogrusFormatter) Format(parent *logrus.Entry) ([]byte, error) {
	entry := &Entry{
		Entry:  parent,
		Level:  FromLogrusLevel(parent.Level),
		Fields: Fields(parent.Data),
	}

	return f.Formatter.Format(entry)
}
