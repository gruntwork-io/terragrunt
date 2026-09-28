package vfs

import "strconv"

// FSKind names the filesystem behind a path, as far as the operating
// system reports it. It exists so callers can size filesystem
// concurrency to the filesystem they are about to touch: the ceiling
// that suits APFS starves overlayfs, and the fan-out overlayfs wants
// collapses btrfs.
type FSKind uint8

const (
	// FSUnknown is a filesystem the platform described but this package
	// does not name.
	FSUnknown FSKind = iota

	// FSUnprobed is a path the platform would not describe, usually one
	// that does not exist yet.
	FSUnprobed

	// FSVirtual is any filesystem other than [NewOSFS].
	FSVirtual

	// FSAPFS is the default on macOS.
	FSAPFS

	// FSHFS is the older macOS filesystem.
	FSHFS

	// FSExt4 is the default on most Linux distributions.
	FSExt4

	// FSXFS is the default on Red Hat Enterprise Linux.
	FSXFS

	// FSBtrfs is the default on Fedora and openSUSE.
	FSBtrfs

	// FSZFS is a copy-on-write filesystem, common on NAS appliances.
	FSZFS

	// FSTmpfs keeps its files in memory.
	FSTmpfs

	// FSOverlay is what a container writes to unless a volume is mounted
	// over it.
	FSOverlay

	// FSFUSE is a filesystem served from userspace, e.g. a Docker Desktop
	// bind mount.
	FSFUSE

	// FSNetwork is storage reached over a network, e.g. NFS, SMB, or 9p.
	FSNetwork

	// FSNTFS is the default on Windows.
	FSNTFS

	// FSReFS is the newer Windows filesystem, used mainly on Windows Server.
	FSReFS

	// fsKindCount bounds the declared set. It must stay last, because
	// [FSKind.String] and [FSKinds] treat it as the number of kinds.
	fsKindCount
)

// fsKindNames is indexed by [FSKind]. Names are the ones the platform
// itself uses, so a log line matches what `mount` prints.
var fsKindNames = [...]string{
	FSUnknown:  "unknown",
	FSUnprobed: "unprobed",
	FSVirtual:  "virtual",
	FSAPFS:     "apfs",
	FSHFS:      "hfs",
	FSExt4:     "ext4",
	FSXFS:      "xfs",
	FSBtrfs:    "btrfs",
	FSZFS:      "zfs",
	FSTmpfs:    "tmpfs",
	FSOverlay:  "overlayfs",
	FSFUSE:     "fuse",
	FSNetwork:  "network",
	FSNTFS:     "ntfs",
	FSReFS:     "refs",
}

// FSKinds returns every declared kind, in declaration order.
func FSKinds() []FSKind {
	kinds := make([]FSKind, 0, fsKindCount)

	for kind := range fsKindCount {
		kinds = append(kinds, kind)
	}

	return kinds
}

// asserts that fsKindNames must hold exactly one entry per kind.
const (
	_ = uint(len(fsKindNames) - int(fsKindCount))
	_ = uint(int(fsKindCount) - len(fsKindNames))
)

// String returns the filesystem's name.
func (k FSKind) String() string {
	if k >= fsKindCount {
		panic("vfs: FSKind out of range: " + strconv.Itoa(int(k)))
	}

	return fsKindNames[k]
}

// DetectFSKind reports the filesystem backing path, using one
// statfs-family syscall.
//
// Returns one of three values:
//
// - a real [FSKind] filesystem
//
// - [FSUnknown] for a filesystem that the Go runtime recognizes, but hasn't
// had performance characteristics measured by Terragrunt.
//
// - [FSUnprobed] for a path the platform declined to describe at all,
// which is usually a sign that [path] doesn't exist on disk.
func DetectFSKind(fsys FS, path string) FSKind {
	if !IsOSFS(fsys) {
		return FSVirtual
	}

	return detectFSKind(path)
}
