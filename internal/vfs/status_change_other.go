//go:build (!linux && !darwin) || mips || mipsle || mips64 || mips64le

package vfs

import "io/fs"

func statusChangeOf(fs.FileInfo) (StatusChange, bool) {
	return StatusChange{}, false
}
