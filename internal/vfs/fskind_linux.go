//go:build linux

package vfs

import "golang.org/x/sys/unix"

// zfsSuperMagic is ZFS's statfs magic. Unlike the filesystems above it,
// x/sys/unix carries no constant for it.
const zfsSuperMagic = 0x2fc12fc1

// fsMagicKinds maps the statfs f_type magic to a kind. The magic numbers
// are kernel ABI, so a missing entry means a filesystem terragrunt has
// no measurement for, not a stale table.
var fsMagicKinds = map[uint32]FSKind{
	unix.EXT4_SUPER_MAGIC:      FSExt4,
	unix.XFS_SUPER_MAGIC:       FSXFS,
	unix.BTRFS_SUPER_MAGIC:     FSBtrfs,
	zfsSuperMagic:              FSZFS,
	unix.TMPFS_MAGIC:           FSTmpfs,
	unix.OVERLAYFS_SUPER_MAGIC: FSOverlay,
	unix.FUSE_SUPER_MAGIC:      FSFUSE,
	unix.NFS_SUPER_MAGIC:       FSNetwork,
	unix.SMB_SUPER_MAGIC:       FSNetwork,
	unix.SMB2_SUPER_MAGIC:      FSNetwork,
	unix.CIFS_SUPER_MAGIC:      FSNetwork,
	unix.V9FS_MAGIC:            FSNetwork,
}

func detectFSKind(path string) FSKind {
	var st unix.Statfs_t

	if err := unix.Statfs(path, &st); err != nil {
		return FSUnprobed
	}

	// A magic with no entry is a real filesystem this package has not
	// measured, which is [FSUnknown] rather than a failure to look.
	return fsMagicKinds[uint32(st.Type)]
}
