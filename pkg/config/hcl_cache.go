package config

import (
	"bytes"
	"context"
	"time"

	"github.com/gruntwork-io/terragrunt/internal/cache"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/pkg/config/hclparse"
	"github.com/gruntwork-io/terragrunt/pkg/log"
)

// CachedHCLFile is a parsed HCL file in the HCL cache.
//
// A stat shows a rewrite only when the rewrite moves the file's mtime. A hit on an entry whose file had a racy mtime
// when it was read compares the file on disk with the bytes the entry was parsed from.
type CachedHCLFile struct {
	*hclparse.File
	modTime time.Time
	source  []byte
	racy    bool
}

// rebindOrParseHCLFile returns cached rebound to a parser from pctx. When cached is nil, it parses the file at path
// and caches it under key, with modTime as the mtime the key was built from.
func rebindOrParseHCLFile(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	pctx *ParsingContext,
	hclCache *cache.Cache[*CachedHCLFile],
	cached *CachedHCLFile,
	key string,
	path string,
	modTime time.Time,
) (*hclparse.File, error) {
	if cached != nil {
		return cached.Rebind(pctx.NewParser(l, v)), nil
	}

	readAt := time.Now()

	file, err := pctx.NewParser(l, v).ParseFromFile(v.FS, path)
	if err != nil {
		return nil, err
	}

	hclCache.Put(ctx, key, &CachedHCLFile{
		File:    file,
		modTime: modTime,
		source:  file.Bytes,
		racy:    isRacyModTime(modTime, readAt),
	})

	return file, nil
}

// cachedHCLFile returns the entry cached under key for the file at path. It returns nil when there is none, or when
// the entry is racy and the file no longer holds the bytes the entry was parsed from.
//
// A racy entry whose file still matches is cached again without the flag once its mtime is outside the window, so
// later hits skip the read.
func cachedHCLFile(
	ctx context.Context,
	fsys vfs.FS,
	hclCache *cache.Cache[*CachedHCLFile],
	key string,
	path string,
) *CachedHCLFile {
	entry, found := hclCache.Get(ctx, key)
	if !found {
		return nil
	}

	if !entry.racy {
		return entry
	}

	readAt := time.Now()

	content, err := vfs.ReadFile(fsys, path)
	if err != nil || !bytes.Equal(content, entry.source) {
		return nil
	}

	if isRacyModTime(entry.modTime, readAt) {
		return entry
	}

	settled := &CachedHCLFile{File: entry.File, modTime: entry.modTime, source: entry.source}
	hclCache.Put(ctx, key, settled)

	return settled
}
