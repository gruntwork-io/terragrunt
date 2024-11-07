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

type logruFormatter struct {
	Formatter
}

func (f *logruFormatter) Format(parent *logrus.Entry) ([]byte, error) {
	entry := &Entry{
		Entry:  parent,
		Level:  FromLogrusLevel(parent.Level),
		Fields: Fields(parent.Data),
	}

	return f.Formatter.Format(entry)
}
