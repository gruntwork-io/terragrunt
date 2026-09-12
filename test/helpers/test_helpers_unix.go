//go:build !windows

package helpers

var RootFolder = "/"

// symlinkPrivilegeError reports whether err is a platform-specific refusal to create a symlink
// for want of a privilege. Only Windows has such an error, so off Windows the portable checks
// in [SymlinkUnsupported] are the whole story.
func symlinkPrivilegeError(error) bool {
	return false
}
