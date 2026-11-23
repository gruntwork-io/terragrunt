//go:build linux && !mips && !mipsle && !mips64 && !mips64le

package vfs

import (
	"io/fs"
	"syscall"
	"time"
)

func statusChangeOf(info fs.FileInfo) (StatusChange, bool) {
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return StatusChange{}, false
	}

	return StatusChange{
		Time:   time.Unix(st.Ctim.Unix()),
		Device: st.Dev,
		Inode:  st.Ino,
	}, true
}
