package vfs

import (
	"errors"
	"hash/maphash"
	"io"
	"time"
)

// racyModTimeWindow is how far a file's mtime can trail the start of a read and still leave a rewrite of the file
// invisible to a later stat. It covers FAT, which records mtimes in 2 second steps.
const racyModTimeWindow = 2 * time.Second

// digestSeed seeds the digests [Digest] and [DigestFile] return. It is random per process.
var digestSeed = maphash.MakeSeed()

// IsRacyModTime reports whether a file with mtime modTime, read at or after readAt, can be rewritten without a later
// stat showing a new mtime.
func IsRacyModTime(modTime, readAt time.Time) bool {
	return !modTime.Before(readAt.Add(-racyModTimeWindow))
}

// Digest returns a digest of content, equal to what [DigestFile] returns for a file that holds content.
//
// Digests of the same contents differ between processes, so a digest is only comparable within the process that
// computed it and must never be persisted.
func Digest(content []byte) uint64 {
	return maphash.Bytes(digestSeed, content)
}

// DigestFile returns a digest of the contents of the file at path, hashing the file as it reads it.
//
// Digests of the same contents differ between processes, so a digest is only comparable within the process that
// computed it and must never be persisted.
func DigestFile(fsys FS, path string) (digest uint64, err error) {
	f, err := fsys.Open(path)
	if err != nil {
		return 0, err
	}

	defer func() {
		err = errors.Join(err, f.Close())
	}()

	var h maphash.Hash

	h.SetSeed(digestSeed)

	if _, err := io.Copy(&h, f); err != nil {
		return 0, err
	}

	return h.Sum64(), nil
}
