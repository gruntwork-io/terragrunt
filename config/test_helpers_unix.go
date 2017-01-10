// +build !windows

package config

var rootFolder = "/root"

func cleanPath(path string) string {
	return strings.Replace(path, "\\", "/", -1)
}
