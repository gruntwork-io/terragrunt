//go:build windows

package vfs

import (
	"strings"

	"golang.org/x/sys/windows"
)

// fsNameKinds maps the filesystem name GetVolumeInformation reports,
// lowercased.
var fsNameKinds = map[string]FSKind{
	"ntfs":     FSNTFS,
	"refs":     FSReFS,
	"nfs":      FSNetwork,
	"webdav":   FSNetwork,
	"cifs":     FSNetwork,
	"9p":       FSNetwork,
	"drvfs":    FSNetwork,
	"virtiofs": FSFUSE,
}

// maxVolumeNameLen bounds both buffers GetVolumeInformation fills.
// MAX_PATH+1 is what the API documents as sufficient for either.
const maxVolumeNameLen = windows.MAX_PATH + 1

func detectFSKind(path string) FSKind {
	pathPtr, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return FSUnprobed
	}

	// GetVolumeInformation describes a volume root, not an arbitrary
	// path, so the mount that holds path has to be resolved first.
	root := make([]uint16, maxVolumeNameLen)
	if err := windows.GetVolumePathName(pathPtr, &root[0], uint32(len(root))); err != nil {
		return FSUnprobed
	}

	fsName := make([]uint16, maxVolumeNameLen)

	err = windows.GetVolumeInformation(
		&root[0],
		nil,
		0,
		nil,
		nil,
		nil,
		&fsName[0],
		uint32(len(fsName)),
	)
	if err != nil {
		return FSUnprobed
	}

	// A name with no entry is a real filesystem this package has not
	// measured, which is [FSUnknown] rather than a failure to look.
	return fsNameKinds[strings.ToLower(windows.UTF16ToString(fsName))]
}
