// +build windows

package helpers

import (
	"fmt"
	"os"
)

var RootFolder = retrieveRootFolder()

func retrieveRootFolder() string {
	cwd, _ := os.Getwd()

	return fmt.Sprintf("%s:/", cwd[0:1])
}
