// +build windows

package config

import "strings"

var rootFolder = "C:/root"

func cleanPath(path string) string {
	return strings.Replace(path, "/", "\\", -1)
}
