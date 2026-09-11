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
	// blob's inode. No content is read or written and the destination adds no
	// disk usage, but the destination is the stored file, so it is
	// materialized without its write bits.
	LinkModeHardlink LinkMode = iota

	// LinkModeClone asks the filesystem for a copy-on-write clone of the
	// stored blob. The destination is a separate file that shares the store's
	// data blocks until it is written, and it keeps the permissions git
	// recorded. APFS, btrfs, and XFS formatted with reflink support offer
	// this; elsewhere the mode copies.
	LinkModeClone

	// LinkModeCopy writes an independent copy of the stored blob with the
	// permissions git recorded. It costs a full read and write per file, and
	// the other modes fall back to it.
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

// String returns the name telemetry reports for the mode.
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

// destPerm returns the permissions of a file materialized under mode. A hard
// link is the stored file, so it loses the write bits and an edit in the
// materialized tree cannot reach the store. A clone or a copy is a separate
// file and keeps the permissions git recorded.
func destPerm(mode LinkMode, gitPerm os.FileMode) os.FileMode {
	perm := gitPerm.Perm()
	if mode != LinkModeHardlink {
		return perm
	}

	return perm &^ WriteBitMask
}

// resolveLinkMode returns the mode that serves a request for mode, given
// whether the caller intends to edit the destination.
//
// A hard link is the stored file itself, so a destination the caller edits is
// cloned instead: a writable file that shares the stored content until it is
// written. That costs one clone per file where the filesystem supports
// copy-on-write, and a copy where it does not. Clones and copies are already
// separate files.
func resolveLinkMode(mode LinkMode, mutable bool) LinkMode {
	if mutable && mode == LinkModeHardlink {
		return LinkModeClone
	}

	return mode
}
