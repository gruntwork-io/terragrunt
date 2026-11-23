//go:build darwin

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
		Time:   time.Unix(st.Ctimespec.Unix()),
		Device: uint64(uint32(st.Dev)),
		Inode:  st.Ino,
	}, true
}
