package util

import (
	"fmt"
	"io"
	"log"
	"os"
)

// Create a logger with the given prefix
func CreateLogger(prefix string) *log.Logger {
	return CreateLoggerWithWriter(os.Stderr, prefix)
}

// CreateLoggerWithWriter Create a lgogger around the given output stream and prefix
func CreateLoggerWithWriter(writer io.Writer, prefix string) *log.Logger {
	if prefix != "" {
		prefix = fmt.Sprintf("[%s] ", prefix)
	}
	return log.New(writer, fmt.Sprintf("[terragrunt] %s", prefix), log.LstdFlags)
}
