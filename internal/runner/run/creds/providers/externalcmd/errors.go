package externalcmd

import "errors"

var (
	// ErrEmptyAuthProviderCmd is returned when parsing the auth provider command
	// yields no command to run, such as a value of only spaces.
	ErrEmptyAuthProviderCmd = errors.New("auth provider command has no command to run")

	// ErrCacheRootUnknown reports a root other than [CacheRootRepo].
	ErrCacheRootUnknown = errors.New("unknown cache root")

	// ErrCacheGlobsEmpty reports a cache object with no globs.
	ErrCacheGlobsEmpty = errors.New("cache globs are empty")

	// ErrCacheGlobsTooMany reports a cache object with more than [MaxCacheGlobs] globs.
	ErrCacheGlobsTooMany = errors.New("too many cache globs")

	// ErrCacheGlobInvalid reports a glob that does not compile.
	ErrCacheGlobInvalid = errors.New("invalid cache glob")

	// ErrCacheGlobsMissUnit reports globs that do not match the unit the command ran for.
	ErrCacheGlobsMissUnit = errors.New("cache globs do not match the unit the command ran for")

	// ErrPreflightKeyUnusable reports a preflight whose cache object cannot be resolved to a key.
	ErrPreflightKeyUnusable = errors.New("auth provider command returned a preflight with an unusable cache key")

	// ErrKeyedResponseIsPreflight reports a request that follows a preflight answered with another preflight.
	ErrKeyedResponseIsPreflight = errors.New("auth provider command answered the request that follows a preflight" +
		" with another preflight")

	// errFlightAbandoned marks a keyed request that failed after the context of the caller that started it ended.
	errFlightAbandoned = errors.New("keyed auth provider request abandoned")
)
