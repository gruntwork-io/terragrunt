//go:build windows

package helpers

import (
	"errors"
	"fmt"
	"os"

	"golang.org/x/sys/windows"
)

var RootFolder = retrieveRootFolder()

func retrieveRootFolder() string {
	cwd, _ := os.Getwd()

	return fmt.Sprintf("%s:/", cwd[0:1])
}

// symlinkPrivilegeError reports whether err is Windows refusing symlink creation because the
// process holds neither SeCreateSymbolicLinkPrivilege nor Developer Mode. Go's
// syscall.Errno.Is does not fold ERROR_PRIVILEGE_NOT_HELD into fs.ErrPermission, so matching
// it portably is not possible and it is checked here directly.
func symlinkPrivilegeError(err error) bool {
	return errors.Is(err, windows.ERROR_PRIVILEGE_NOT_HELD)
}
