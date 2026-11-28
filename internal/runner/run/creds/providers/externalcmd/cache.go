package externalcmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"path/filepath"
	"slices"
	"sync"
	"time"

	"github.com/gruntwork-io/terragrunt/internal/git"
	"github.com/gruntwork-io/terragrunt/internal/glob"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"golang.org/x/sync/singleflight"
)

const (
	// EnvSchema is the environment variable that tells an auth provider command which response schema Terragrunt
	// reads. Terragrunt sets it to [SchemaVersion] on every invocation that honors the cache object.
	EnvSchema = "TG_CTX_AUTH_PROVIDER_SCHEMA"

	// EnvCacheKey is the environment variable that Terragrunt sets to the resolved cache key, as JSON, on the
	// request that follows a preflight. Terragrunt removes it from every other invocation that honors the cache
	// object.
	EnvCacheKey = "TG_CTX_AUTH_PROVIDER_CACHE_KEY"

	// SchemaVersion is the response schema version Terragrunt announces in [EnvSchema].
	SchemaVersion = "v3"

	// CacheRootRepo makes the globs of a [Cache] relative to the top level of the git repository that contains the
	// unit.
	CacheRootRepo CacheRoot = "repo"

	// MaxCacheGlobs is the most patterns a [Cache] may list.
	MaxCacheGlobs = 256

	// MaxCacheEntries is the most live responses a [ResponseCache] keeps.
	MaxCacheEntries = 4096

	// MaxCacheTTL is the longest a response is reused. A longer ttl is shortened to it.
	MaxCacheTTL = 24 * time.Hour

	// maxCacheTTLSeconds is [MaxCacheTTL] in the unit of a ttl.
	maxCacheTTLSeconds = int64(MaxCacheTTL / time.Second)

	// maxCompiledGlobs is the most compiled patterns a [ResponseCache] remembers.
	maxCompiledGlobs = 4096

	// maxFlightAttempts is how many keyed requests a caller joins before it fails, when each one is abandoned by
	// the caller that started it.
	maxFlightAttempts = 3
)

// CacheRoot is the kind of directory the globs of a [Cache] are relative to.
type CacheRoot string

// Cache is the JSON schema for the cache object of a [Response]. Terragrunt reuses the response for every unit
// whose directory matches Globs.
type Cache struct {
	// Root is the directory Globs are relative to.
	Root CacheRoot `json:"root" jsonschema:"required,enum=repo"`
	// Globs are patterns in the syntax of a --filter path, matched against the unit directory relative to Root,
	// with forward slashes.
	Globs []string `json:"globs" jsonschema:"required,minItems=1,maxItems=256"`
	// TTL is how many seconds the response may be reused, up to [MaxCacheTTL]. Zero disables caching. A negative
	// value marks the response as a preflight: Root and Globs are the cache key, and the response has no credentials.
	TTL int64 `json:"ttl" jsonschema:"required,maximum=86400"`
}

// ResponseCache stores auth provider command responses that have a [Cache], and returns one for a unit whose
// directory matches its globs. It is safe for concurrent use.
type ResponseCache struct {
	flights       singleflight.Group
	compiledGlobs map[string]glob.Matcher
	entries       []cacheEntry
	mu            sync.RWMutex
}

// NewResponseCache returns an empty [ResponseCache].
func NewResponseCache() *ResponseCache {
	return &ResponseCache{
		compiledGlobs: make(map[string]glob.Matcher),
	}
}

// ContextWithResponseCache returns a copy of ctx with a fresh [ResponseCache], which every auth provider command
// run under that context shares.
func ContextWithResponseCache(ctx context.Context) context.Context {
	return context.WithValue(ctx, responseCacheContextKey{}, NewResponseCache())
}

type responseCacheContextKey struct{}

// cacheEntry is a response stored under a [cacheKey] until it expires.
type cacheEntry struct {
	expires time.Time
	resp    *Response
	root    string
	id      string
	globs   []glob.Matcher
}

// cacheKey is a [Cache] resolved for one unit. Its id, the JSON a keyed request receives in [EnvCacheKey],
// includes root.
type cacheKey struct {
	root  string
	id    string
	globs []glob.Matcher
}

// unitPath is a unit directory split into its repository root and the slash-separated path below it.
type unitPath struct {
	root string
	rel  string
}

// responseCacheFromContext returns the cache stored by [ContextWithResponseCache].
//
// Panics when ctx has none. [github.com/gruntwork-io/terragrunt/internal/cli/commands.RunAction] installs one for
// every command. Verified safe in TestAuthProviderCacheReachedFromEveryCommand, in internal/cli.
func responseCacheFromContext(ctx context.Context) *ResponseCache {
	c, ok := ctx.Value(responseCacheContextKey{}).(*ResponseCache)
	if !ok {
		panic("auth provider response cache is not installed on the context")
	}

	return c
}

// lookup returns a live response whose globs match unit. When several entries match, the oldest wins.
func (c *ResponseCache) lookup(unit unitPath) (*Response, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	now := time.Now()

	for _, entry := range c.entries {
		if entry.root != unit.root || !entry.expires.After(now) {
			continue
		}

		if matchesAny(entry.globs, unit.rel) {
			return entry.resp, true
		}
	}

	return nil, false
}

// get returns the live response stored under key.
func (c *ResponseCache) get(key cacheKey) (*Response, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	now := time.Now()

	for _, entry := range c.entries {
		if entry.id == key.id && entry.expires.After(now) {
			return entry.resp, true
		}
	}

	return nil, false
}

// put stores resp under key for ttl seconds, replacing an entry with the same key.
//
// Reports false, storing nothing, when the cache already has [MaxCacheEntries] live entries.
func (c *ResponseCache) put(key cacheKey, resp *Response, ttl int64) bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()

	c.entries = slices.DeleteFunc(c.entries, func(entry cacheEntry) bool {
		return entry.id == key.id || !entry.expires.After(now)
	})

	if len(c.entries) >= MaxCacheEntries {
		return false
	}

	c.entries = append(c.entries, cacheEntry{
		expires: now.Add(time.Duration(ttl) * time.Second),
		resp:    resp,
		root:    key.root,
		id:      key.id,
		globs:   key.globs,
	})

	return true
}

// flight runs fetch once for all concurrent callers that pass the same key, and returns its result to each. A caller
// whose context ends stops waiting. When the caller running fetch is cancelled, a waiting caller runs it again.
//
// Returns an error once maxFlightAttempts requests the caller joined were cancelled that way.
func (c *ResponseCache) flight(
	ctx context.Context,
	key cacheKey,
	fetch func() (*Response, error),
) (*Response, error) {
	var abandoned error

	for range maxFlightAttempts {
		ch := c.flights.DoChan(key.id, func() (any, error) {
			resp, err := fetch()
			if err != nil && ctx.Err() != nil {
				return nil, errors.Join(errFlightAbandoned, err)
			}

			return resp, err
		})

		select {
		case <-ctx.Done():
			return nil, context.Cause(ctx)
		case res := <-ch:
			if res.Err == nil {
				resp, ok := res.Val.(*Response)
				if !ok {
					panic(fmt.Sprintf("keyed auth provider request returned %T, want *Response", res.Val))
				}

				return resp, nil
			}

			if ctx.Err() != nil || !errors.Is(res.Err, errFlightAbandoned) {
				return nil, res.Err
			}

			abandoned = res.Err
		}
	}

	return nil, fmt.Errorf("giving up after %d attempts: %w", maxFlightAttempts, abandoned)
}

// resolveUnitPath splits dir into the git repository root that contains it and its path below that root.
func resolveUnitPath(ctx context.Context, v *venv.Venv, dir string) (unitPath, error) {
	root, err := git.GoRepoRoot(ctx, v, dir)
	if err != nil {
		return unitPath{}, err
	}

	rel, err := filepath.Rel(root, vfs.ResolveForCompare(v.FS, filepath.Clean(dir)))
	if err != nil {
		return unitPath{}, err
	}

	return unitPath{root: root, rel: filepath.ToSlash(rel)}, nil
}

// compile returns the matcher for pattern, compiling it the first time the cache sees it. Past maxCompiledGlobs
// remembered patterns, a new one is compiled on every call.
func (c *ResponseCache) compile(l log.Logger, pattern string) (glob.Matcher, error) {
	c.mu.RLock()
	compiled, ok := c.compiledGlobs[pattern]
	c.mu.RUnlock()

	if ok {
		return compiled, nil
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if compiled, ok := c.compiledGlobs[pattern]; ok {
		return compiled, nil
	}

	compiled, err := glob.Compile(path.Clean(filepath.ToSlash(pattern)))
	if err != nil {
		return nil, err
	}

	if len(c.compiledGlobs) >= maxCompiledGlobs {
		l.Debugf("Not remembering the compiled cache glob %q: the cache already remembers %d globs.",
			pattern, maxCompiledGlobs)

		return compiled, nil
	}

	c.compiledGlobs[pattern] = compiled

	return compiled, nil
}

// newKey validates cache and resolves it for unit.
func (c *ResponseCache) newKey(l log.Logger, cache *Cache, unit unitPath) (cacheKey, error) {
	if cache.Root != CacheRootRepo {
		return cacheKey{}, fmt.Errorf("%w %q", ErrCacheRootUnknown, cache.Root)
	}

	if len(cache.Globs) == 0 {
		return cacheKey{}, ErrCacheGlobsEmpty
	}

	if len(cache.Globs) > MaxCacheGlobs {
		return cacheKey{}, fmt.Errorf("%w: %d, limit %d", ErrCacheGlobsTooMany, len(cache.Globs), MaxCacheGlobs)
	}

	globs := make([]glob.Matcher, 0, len(cache.Globs))

	for _, pattern := range cache.Globs {
		compiled, err := c.compile(l, pattern)
		if err != nil {
			return cacheKey{}, fmt.Errorf("%w %q: %w", ErrCacheGlobInvalid, pattern, err)
		}

		globs = append(globs, compiled)
	}

	// Caching the response would serve other units while this unit reruns the command on every fetch.
	if !matchesAny(globs, unit.rel) {
		return cacheKey{}, fmt.Errorf("%w: %q", ErrCacheGlobsMissUnit, unit.rel)
	}

	id, err := json.Marshal(struct {
		Root  string   `json:"root"`
		Globs []string `json:"globs"`
	}{
		Root:  unit.root,
		Globs: cache.Globs,
	})
	if err != nil {
		return cacheKey{}, fmt.Errorf("encoding cache key: %w", err)
	}

	return cacheKey{root: unit.root, id: string(id), globs: globs}, nil
}

// matchesAny reports whether rel matches one of globs.
func matchesAny(globs []glob.Matcher, rel string) bool {
	return slices.ContainsFunc(globs, func(g glob.Matcher) bool {
		return g.Match(rel)
	})
}

// requestVenv returns a copy of v whose environment sets [EnvSchema] to [SchemaVersion] and [EnvCacheKey] to keyID.
// An empty keyID removes the variable, so a key inherited from the process environment does not turn the request
// into a keyed one.
func requestVenv(v *venv.Venv, keyID string) *venv.Venv {
	rv := v.WithEnvCloned()
	rv.Env[EnvSchema] = SchemaVersion

	delete(rv.Env, EnvCacheKey)

	if keyID != "" {
		rv.Env[EnvCacheKey] = keyID
	}

	return rv
}
