//go:build windows

package helpers

import (
	"fmt"
	"os"
)

// RootFolder is the root of the drive holding the working directory.
var RootFolder = retrieveRootFolder()

func retrieveRootFolder() string {
	cwd, _ := os.Getwd()

	return fmt.Sprintf("%s:/", cwd[0:1])
}
