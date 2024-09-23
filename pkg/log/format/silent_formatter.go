package format

import (
	"github.com/sirupsen/logrus"
)

// SilentFormatter disables logging by not outputting anything.
type SilentFormatter struct{}

// Format implements logrus.Formatter interface.
func (f *SilentFormatter) Format(entry *logrus.Entry) ([]byte, error) {
	return nil, nil
}
