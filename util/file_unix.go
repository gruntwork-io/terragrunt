//go:build !windows
// +build !windows

package util

import (
	"errors"
	"os"
	"os/user"
)

// HomeDir returns the path to the home directory.
func HomeDir() (string, error) {
	// First prefer the HOME environmental variable
	if home := os.Getenv("HOME"); home != "" {
		return home, nil
	}

	// If that fails, try built-in module
	user, err := user.Current()
	if err != nil {
		return "", err
	}

	if user.HomeDir == "" {
		return "", errors.New("blank output")
	}

	return user.HomeDir, nil
}
