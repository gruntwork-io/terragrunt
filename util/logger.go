package util

import (
	"os"
	"log"
	"fmt"
)

// Create a logger with the given prefix
func CreateLogger(prefix string) *log.Logger {
	if prefix != "" {
		prefix = fmt.Sprintf("[%s] ", prefix)
	}
	return log.New(os.Stdout, fmt.Sprintf("[terragrunt] %s", prefix), log.LstdFlags)
}