package config

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"maps"
	"slices"
	"strconv"
	"strings"
	"sync/atomic"

	"github.com/gruntwork-io/terragrunt/internal/cache"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/pkg/config/hclparse"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/huandu/go-clone"
	"github.com/zclconf/go-cty/cty"
)

// EnvDisableBaseBlocksCache turns the base blocks cache off, leaving every unit to decode
// the parents it includes for itself. It is a temporary, undocumented environment variable
// used while this cache is being evaluated. Do NOT rely on it: it can be removed or have
// its name changed at any time without notice, and it is not part of Terragrunt's
// user-facing configuration surface.
const EnvDisableBaseBlocksCache = "TG_TMP_DISABLE_BASE_BLOCKS_CACHE"

// baseBlocksCacheKeySeparator separates the parts of a base blocks cache key. It cannot
// appear in a path, a digest, or an environment variable name.
const baseBlocksCacheKeySeparator = "\x00"

// baseBlocksCacheLocalsRoots are the variable prefixes a cached locals block may read.
// local resolves within the block's own fixed point, and feature resolves to the flags the
// same file declares plus the run's CLI overrides. Every other prefix (values, dependency,
// include) is filled in from the unit currently being parsed.
var baseBlocksCacheLocalsRoots = map[string]bool{
	MetadataLocal:       true,
	MetadataFeatureFlag: true,
}

// BaseBlocksCache memoizes the child-independent result of [DecodeBaseBlocks], so a parent
// shared by many units is decoded once per run instead of once per unit per decode list.
//
// Entries stay in memory for the life of the process and nothing here writes one out. An
// entry can hold secret material: get_env is cacheable and the environment carries cloud
// credentials. Terragrunt persists none of that today, and a disk-backed store here would
// change that, so this cache must remain memory-backed and must never serialize an entry.
type BaseBlocksCache struct {
	entries *cache.Cache[*baseBlocksCacheEntry]
	hits    atomic.Int64
	misses  atomic.Int64
}

// baseBlocksCacheEntry holds the part of [DecodedBaseBlocks] that cannot vary with the unit
// a parent is being decoded for. TrackInclude is left out: it carries the child's own
// include block, and is cheap enough to rebuild on every hit.
type baseBlocksCacheEntry struct {
	// Locals is the evaluated locals block, as the eval context sees it.
	Locals *cty.Value
	// FeatureFlags is the feature flag map, as the eval context sees it.
	FeatureFlags *cty.Value
}

// NewBaseBlocksCache returns an empty cache. The name prefixes its telemetry counters.
func NewBaseBlocksCache(name string) *BaseBlocksCache {
	return &BaseBlocksCache{entries: cache.NewCache[*baseBlocksCacheEntry](name)}
}

// ContextBaseBlocksCache returns the cache installed by [WithConfigValues], or a detached
// one when the context carries none, matching how the other config caches are reached.
func ContextBaseBlocksCache(ctx context.Context) *BaseBlocksCache {
	if c, ok := ctx.Value(BaseBlocksCacheContextKey).(*BaseBlocksCache); ok && c != nil {
		return c
	}

	return NewBaseBlocksCache(baseBlocksCacheName)
}

// Hits returns how many decodes this cache has answered.
func (c *BaseBlocksCache) Hits() int64 {
	return c.hits.Load()
}

// Misses returns how many decodes this cache could not answer. Since a miss is followed by
// exactly one decode of the file, this is the number of times a cacheable file was decoded.
func (c *BaseBlocksCache) Misses() int64 {
	return c.misses.Load()
}

// get returns a deep copy of the entry stored under key, so a caller that goes on to
// modify what it reads cannot reach the shared entry.
func (c *BaseBlocksCache) get(ctx context.Context, key string) (*baseBlocksCacheEntry, bool) {
	entry, found := c.entries.Get(ctx, key)
	if !found {
		c.misses.Add(1)

		return nil, false
	}

	c.hits.Add(1)

	return clone.Clone(entry).(*baseBlocksCacheEntry), true
}

// put stores a deep copy of entry, so the caller keeps sole ownership of what it decoded.
func (c *BaseBlocksCache) put(ctx context.Context, key string, entry *baseBlocksCacheEntry) {
	c.entries.Put(ctx, key, clone.Clone(entry).(*baseBlocksCacheEntry))
}

// baseBlocksCacheKey returns the key the base blocks of file are cached under, and whether
// they may be cached at all.
//
// The key names the file and its content, and nothing about the unit being parsed: the
// point of the cache is that one parent answers the same for every child, so a key holding
// the decode list or the child's path would defeat it. Caching is refused outright unless
// every expression that feeds the entry is provably independent of the child, which is what
// the walk below establishes.
func baseBlocksCacheKey(pctx *ParsingContext, file *hclparse.File) (string, bool) {
	if baseBlocksCacheDisabled(pctx.Venv.Env) {
		return "", false
	}

	// A predefined function is installed for one parse and shadows the table the
	// classification was derived from, so nothing in the file can be judged against it.
	if len(pctx.PredefinedFunctions) > 0 {
		return "", false
	}

	// A diagnostics handler decides which diagnostics become errors and may drop one
	// outright, so a decode running under one can succeed with the locals that survived
	// suppression. Discovery installs such a handler and shares this cache with the run's own
	// parses, which would then be served a truncated entry instead of the file's error.
	if file.HasDiagnosticsHandler() {
		return "", false
	}

	// A JSON config has no hclsyntax AST to walk, so its expressions cannot be classified.
	body, ok := file.Body.(*hclsyntax.Body)
	if !ok {
		return "", false
	}

	usage := &childIndependenceUsage{}
	if !baseBlocksAreChildIndependent(body, usage) {
		return "", false
	}

	key := file.ConfigPath + baseBlocksCacheKeySeparator + contentDigest(file.Bytes)

	// Dependency output fetching can add environment keys part way through a run, so a file
	// that reads the environment is keyed on it rather than assumed to be run-global.
	if usage.readsEnv {
		key += baseBlocksCacheKeySeparator + envDigest(pctx.Venv.Env)
	}

	return key, true
}

// baseBlocksAreChildIndependent reports whether the blocks [DecodeBaseBlocks] evaluates
// hold nothing that could differ between the units a parent is decoded for.
func baseBlocksAreChildIndependent(body *hclsyntax.Body, usage *childIndependenceUsage) bool {
	// An included file's locals, feature defaults and merge order all arrive through the
	// parsing context, and a file that includes another is a unit rather than the shared
	// parent this cache exists for. Every unit lands here, so rule that out before walking any
	// expressions.
	for _, block := range body.Blocks {
		if block.Type == MetadataInclude {
			return false
		}
	}

	for _, block := range body.Blocks {
		switch block.Type {
		case MetadataLocals:
			if !bodyIsChildIndependent(block.Body, baseBlocksCacheLocalsRoots, usage, 0) {
				return false
			}
		case MetadataFeatureFlag:
			// Feature defaults are decoded before this file's own locals are evaluated, so a
			// variable here reads the child's locals and values, never the parent's.
			if !bodyIsChildIndependent(block.Body, nil, usage, 0) {
				return false
			}
		}
	}

	return true
}

// baseBlocksCacheDisabled reports whether the kill switch is set. Setting it at all means
// turning the cache off, so a value that is not a boolean disables it too.
func baseBlocksCacheDisabled(env map[string]string) bool {
	venv.RequireEnvMap(env)

	raw := strings.TrimSpace(env[EnvDisableBaseBlocksCache])
	if raw == "" {
		return false
	}

	disabled, err := strconv.ParseBool(raw)
	if err != nil {
		return true
	}

	return disabled
}

// contentDigest returns a hex digest of a config file's content.
func contentDigest(content []byte) string {
	sum := sha256.Sum256(content)

	return hex.EncodeToString(sum[:])
}

// envDigest returns a hex digest of the whole environment, ordered so two contexts holding
// the same variables produce the same digest.
func envDigest(env map[string]string) string {
	entries := make([]string, 0, len(env))

	for _, name := range slices.Sorted(maps.Keys(env)) {
		entries = append(entries, name+"="+env[name])
	}

	return contentDigest([]byte(strings.Join(entries, baseBlocksCacheKeySeparator)))
}
