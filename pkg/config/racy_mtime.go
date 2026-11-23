package config

import (
	"hash/maphash"
	"time"

	"github.com/gruntwork-io/terragrunt/internal/vfs"
)

// racyModTimeWindow is how far a file's mtime can trail the start of a read and still leave a rewrite of the file
// invisible to a later stat. It covers FAT, which records mtimes in 2 second steps.
const racyModTimeWindow = 2 * time.Second

// fileDigestSeed seeds the digests [digestFile] returns. It is random per process.
var fileDigestSeed = maphash.MakeSeed()

// isRacyModTime reports whether a file with mtime modTime, read at or after readAt, can be rewritten without a later
// stat showing a new mtime.
func isRacyModTime(modTime, readAt time.Time) bool {
	return !modTime.Before(readAt.Add(-racyModTimeWindow))
}

// digestBytes returns the digest [digestFile] returns for a file holding content.
func digestBytes(content []byte) uint64 {
	return maphash.Bytes(fileDigestSeed, content)
}

// digestFile returns a digest of the contents of the file at path.
//
// Digests of the same contents differ between processes, so a digest is only comparable within the process that
// computed it and must never be persisted.
func digestFile(fsys vfs.FS, path string) (uint64, error) {
	var h maphash.Hash

	h.SetSeed(fileDigestSeed)

	if err := vfs.HashFile(fsys, &h, path); err != nil {
		return 0, err
	}

	return h.Sum64(), nil
}
