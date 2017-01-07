package util

import (
	"fmt"
	"log"
	"os"
)

// Create a logger with the given prefix
func CreateLogger(prefix string) *log.Logger {
	if prefix != "" {
		prefix = fmt.Sprintf("[%s] ", prefix)
	}
	return log.New(os.Stderr, fmt.Sprintf("[terragrunt] %s", prefix), log.LstdFlags)
}
