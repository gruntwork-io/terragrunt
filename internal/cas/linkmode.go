package cas

import (
	"fmt"
	"os"
)

// LinkMode selects how a stored blob reaches its destination when a tree is
// materialized.
type LinkMode int

const (
	// LinkModeHardlink gives the destination a second name for the stored
	// blob's inode. No content is read or written, and the destination adds
	// no disk usage, at the cost of handing out a file the store shares: it
	// is materialized without its write bits, and a source marked mutable is
	// served by [LinkModeClone] instead.
	LinkModeHardlink LinkMode = iota

	// LinkModeClone asks the filesystem for a copy-on-write clone of the
	// stored blob. The destination is a separate file that shares the
	// store's data blocks until it is written, so it keeps the permissions
	// git recorded and a mutable source costs no more than an immutable one.
	// APFS, btrfs, and XFS formatted with reflink support offer this;
	// elsewhere the mode falls back to a hard link and then to a copy.
	LinkModeClone

	// LinkModeCopy writes an independent copy of the stored blob carrying
	// the permissions git recorded. It costs a full read and write per file
	// and is the mode every other one degrades to.
	LinkModeCopy
)

// DefaultLinkMode is the mode CAS materializes trees in when nothing selects
// another one.
const DefaultLinkMode = LinkModeHardlink

// linkModeNames maps each [LinkMode] to the name telemetry reports.
var linkModeNames = map[LinkMode]string{
	LinkModeHardlink: "hardlink",
	LinkModeClone:    "clone",
	LinkModeCopy:     "copy",
}

// String returns the mode's flag name.
func (m LinkMode) String() string {
	name, ok := linkModeNames[m]
	if !ok {
		return fmt.Sprintf("LinkMode(%d)", int(m))
	}

	return name
}

// InvalidLinkModeError reports a [LinkMode] outside the declared set,
// which only an internal caller can produce.
type InvalidLinkModeError struct {
	// Value is the rejected mode, rendered by [LinkMode.String].
	Value string
}

func (e *InvalidLinkModeError) Error() string {
	return fmt.Sprintf("invalid CAS link mode %q", e.Value)
}

// sharesStoredFile reports whether the mode hands the destination the
// store's own file rather than a file of its own.
func (m LinkMode) sharesStoredFile() bool {
	return m == LinkModeHardlink
}

// destPerm returns the permissions a file materialized under mode carries.
//
// A mode that shares the stored file loses the write bits, so an edit in the
// materialized tree cannot reach the shared store. A mode that produces a
// file of its own keeps the permissions git recorded, which is what a mutable
// source always ends up under: [resolveLinkMode] has already turned such a
// request into a mode that shares nothing.
func destPerm(mode LinkMode, gitPerm os.FileMode) os.FileMode {
	perm := gitPerm.Perm()
	if !mode.sharesStoredFile() {
		return perm
	}

	return perm &^ WriteBitMask
}

// resolveLinkMode returns the mode that can serve a request for mode on a
// tree the caller intends to mutate.
//
// A hard link would hand out the store's own inode, so a mutable tree is
// cloned instead: the destination is a writable file of its own that shares
// the stored content until it is written, which costs nothing where the
// filesystem offers copy-on-write clones and degrades to a copy where it
// does not. Every other mode already produces a file of its own.
func resolveLinkMode(mode LinkMode, forceCopy bool) LinkMode {
	if forceCopy && mode == LinkModeHardlink {
		return LinkModeClone
	}

	return mode
}
