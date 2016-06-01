package util

import "os"

// Return true if the given file exists
func FileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
