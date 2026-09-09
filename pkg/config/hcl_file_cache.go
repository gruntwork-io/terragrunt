package config

import (
	"context"
	"errors"
	"io/fs"
	"sync/atomic"

	"github.com/gruntwork-io/terragrunt/internal/cache"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/pkg/config/hclparse"
)

// hclFileCacheKeySeparator separates the parts of an HCL file cache key. It cannot appear in
// a path or in a hex digest.
const hclFileCacheKeySeparator = "\x00"

// HCLFileCache memoizes the AST a config file parses to, so a file with many readers, such as
// a parent every unit includes, is lexed once per run rather than once per reader.
//
// Entries stay in memory for the life of the process. [HCLFileCacheKey] describes what
// identifies one; a parse that raised diagnostics is not stored under any key.
type HCLFileCache struct {
	entries *cache.Cache[*hclparse.File]
	hits    atomic.Int64
	misses  atomic.Int64
}

// NewHCLFileCache returns an empty cache. The name prefixes its telemetry counters.
func NewHCLFileCache(name string) *HCLFileCache {
	return &HCLFileCache{entries: cache.NewCache[*hclparse.File](name)}
}

// ContextHCLFileCache returns the cache installed by [WithConfigValues], or a detached one
// when the context carries none, matching how the other config caches are reached.
func ContextHCLFileCache(ctx context.Context) *HCLFileCache {
	if c, ok := ctx.Value(HclCacheContextKey).(*HCLFileCache); ok && c != nil {
		return c
	}

	return NewHCLFileCache(hclCacheName)
}

// HCLFileCacheKey returns the key the AST of the file at configPath holding content is stored
// under.
//
// A parse answers to the path and the content, and to nothing else. The path chooses the JSON
// or the HCL parser and is written into every source range the AST carries; the content is the
// rest. Parser options decide what becomes of the diagnostics a parse raises, never what a
// parse that raises none produces, so the entries they endanger are refused rather than keyed
// on them. Nothing about the reader belongs here: one file read by the full and the partial
// entry point, for any unit and any decode list, is one AST.
func HCLFileCacheKey(configPath string, content []byte) string {
	return configPath + hclFileCacheKeySeparator + contentDigest(content)
}

// Hits returns how many lookups this cache answered.
func (c *HCLFileCache) Hits() int64 {
	return c.hits.Load()
}

// Misses returns how many lookups this cache could not answer. A parse follows every miss an
// entry point takes, so this is how many times a file was parsed.
func (c *HCLFileCache) Misses() int64 {
	return c.misses.Load()
}

// Get returns the AST stored under key. It is shared with every other reader of that file, so
// a caller that goes on to decode it binds it to a parser of its own first with
// [hclparse.File.Rebind].
func (c *HCLFileCache) Get(ctx context.Context, key string) (*hclparse.File, bool) {
	file, found := c.entries.Get(ctx, key)
	if !found {
		c.misses.Add(1)

		return nil, false
	}

	c.hits.Add(1)

	return file, true
}

// put stores what file holds, bound to a parser of the cache's own.
//
// The first caller to decode a file carrying a bare `include {}` rewrites the wrapper it is
// holding: [hclparse.File.Update] swaps both the AST and the parser behind it. Storing the
// caller's own wrapper would therefore replace the entry underneath every later reader, and
// race with the readers already decoding it.
//
// A parse that raised diagnostics is not stored at all. Without a diagnostics handler such a
// parse fails, so a file that holds diagnostics reached its caller only because a handler
// dropped them, and it holds whatever the parser recovered rather than what the file says.
// Discovery installs such a handler by default and shares this cache with the run's own parses,
// which would then be served that recovered AST instead of the file's error. A parse that raised
// nothing is the parse any reader would have made, whatever options it holds, so discovery's
// reads of a sound file still serve the run.
func (c *HCLFileCache) put(ctx context.Context, key string, file *hclparse.File) {
	if file.HasDiagnostics() {
		return
	}

	c.entries.Put(ctx, key, file.Rebind(hclparse.NewParser()))
}

// hclFileLookup is a config file's content paired with what the HCL file cache holds for it.
//
// The lookup is separate from the parse so an entry point can report the cache outcome on the
// span it opens around the parse without looking the file up a second time.
type hclFileLookup struct {
	// cache is the cache the lookup was made against, and the one a parse is stored into.
	cache *HCLFileCache
	// parser is the parser the file this lookup resolves to is bound to.
	parser *hclparse.Parser
	// cached is the stored AST, bound to parser, or nil when the cache held none.
	cached *hclparse.File
	// configPath is the path content was read from.
	configPath string
	// key is the key the path and the content map to.
	key string
	// content is the file as it was read.
	content []byte
}

// lookupHCLFile looks the content read from configPath up in the run's HCL file cache, binding
// whatever it finds to parser.
func lookupHCLFile(
	ctx context.Context,
	configPath string,
	content []byte,
	parser *hclparse.Parser,
) *hclFileLookup {
	entries := ContextHCLFileCache(ctx)

	lookup := &hclFileLookup{
		cache:      entries,
		parser:     parser,
		configPath: configPath,
		key:        HCLFileCacheKey(configPath, content),
		content:    content,
	}

	if cached, found := entries.Get(ctx, lookup.key); found {
		lookup.cached = cached.Rebind(parser)
	}

	return lookup
}

// resolve returns the AST of the looked up file, parsing the content it already holds and
// caching the result when the lookup found nothing.
func (lookup *hclFileLookup) resolve(ctx context.Context) (*hclparse.File, error) {
	if lookup.cached != nil {
		return lookup.cached, nil
	}

	file, err := lookup.parser.ParseFromBytes(lookup.content, lookup.configPath)
	if err != nil {
		return nil, err
	}

	lookup.cache.put(ctx, lookup.key, file)

	return file, nil
}

// readConfigFile returns the content of the config file at configPath, reporting a file that is
// not there as [TerragruntConfigNotFoundError] so callers can tell it from a failed read.
func readConfigFile(fsys vfs.FS, configPath string) ([]byte, error) {
	content, err := vfs.ReadFile(fsys, configPath)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, TerragruntConfigNotFoundError{Path: configPath}
		}

		return nil, err
	}

	return content, nil
}
