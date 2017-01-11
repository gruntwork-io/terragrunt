// +build !windows

package helpers

var RootFolder = "/"

func CleanPath(path string) string {
	return strings.Replace(path, `\`, "/", -1)
}

func CleanHclPath(path string) string {
	return strings.Replace(path, `\\`, "/", -1)
}