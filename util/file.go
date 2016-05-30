package util

import (
	"os"
	"io/ioutil"
)

// Return true if the given path exists
func PathExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// Returns true if the given path is a directory
func IsDir(path string) (bool, error) {
	stat, err := os.Stat(path)
	switch {
	case err != nil: return false, err
	case stat.IsDir(): return true, nil
	default: return false, nil
	}
}

// Returns true if the given directory is empty
func IsDirEmpty(dir string) (bool, error) {
	files, err := ioutil.ReadDir(dir)
	if err != nil {
		return false, err
	}
	return len(files) == 0, nil
}