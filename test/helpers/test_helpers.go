package helpers

import "runtime"

func IsWindows() bool {
	return runtime.GOOS == "windows"
}
