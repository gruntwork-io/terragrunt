//go:build darwin

package vfs

import "golang.org/x/sys/unix"

// fsTypeNameKinds maps the name darwin's statfs reports in
// f_fstypename. Darwin names the filesystem outright, so there is no
// magic-number table to keep in step with the kernel.
var fsTypeNameKinds = map[string]FSKind{
	"apfs":   FSAPFS,
	"hfs":    FSHFS,
	"ntfs":   FSNTFS,
	"nfs":    FSNetwork,
	"smbfs":  FSNetwork,
	"afpfs":  FSNetwork,
	"webdav": FSNetwork,
}

func detectFSKind(path string) FSKind {
	var st unix.Statfs_t

	if err := unix.Statfs(path, &st); err != nil {
		return FSUnprobed
	}

	name := unix.ByteSliceToString(st.Fstypename[:])

	// A name with no entry is a real filesystem this package has not
	// measured, which is [FSUnknown] rather than a failure to look.
	return fsTypeNameKinds[name]
}
