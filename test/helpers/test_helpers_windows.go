// +build windows

package helpers

import "strings"

var RootFolder = "C:/"

// converts slashes to backslashes for path comparisons
func CleanPath(path string) string {
	return strings.Replace(path, "/", `\`, -1)
}

// converts slashes to escaped backslashes for HCL path comparisons
func CleanHclPath(path string) string {
	return strings.Replace(path, "/", `\\`, -1)
}
