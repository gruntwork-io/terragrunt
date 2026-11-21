package config

import (
	"errors"
	"hash/maphash"
	"io"
	"sync"
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

// digestFile returns a digest of the contents of the file at path, hashing the file as it reads it through a buffer
// from digestBuffers.
//
// Digests of the same contents differ between processes, so a digest is only comparable within the process that
// computed it and must never be persisted.
func digestFile(fsys vfs.FS, path string) (digest uint64, err error) {
	f, err := fsys.Open(path)
	if err != nil {
		return 0, err
	}

	defer func() {
		err = errors.Join(err, f.Close())
	}()

	buf := digestBuffers.Get().(*[]byte)
	defer digestBuffers.Put(buf)

	var h maphash.Hash

	h.SetSeed(fileDigestSeed)

	// io.CopyBuffer ignores buf for a reader with a WriteTo method, and *os.File's allocates a buffer of its own on
	// every call.
	if _, err := io.CopyBuffer(&h, struct{ io.Reader }{f}, *buf); err != nil {
		return 0, err
	}

	return h.Sum64(), nil
}

// digestBufferSize is the size of the buffers digestFile reads through.
const digestBufferSize = 32 * 1024

// digestBuffers holds the buffers digestFile reads through, so a digest allocates none.
var digestBuffers = sync.Pool{
	New: func() any {
		buf := make([]byte, digestBufferSize)

		return &buf
	},
}
