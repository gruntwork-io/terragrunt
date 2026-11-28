package externalcmd_test

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/gruntwork-io/terragrunt/internal/experiment"
	"github.com/gruntwork-io/terragrunt/internal/git"
	"github.com/gruntwork-io/terragrunt/internal/runner/run/creds/providers"
	"github.com/gruntwork-io/terragrunt/internal/runner/run/creds/providers/externalcmd"
	"github.com/gruntwork-io/terragrunt/internal/shell"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/internal/vexec"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sync/errgroup"
)

const (
	cacheCmd         = "auth"
	cacheAccessKeyID = "fake-access-key-id"
)

// TestProviderIgnoresCacheWithoutExperiment pins that the cache object does nothing while response caching is
// disabled: every fetch runs the command, with no schema variable.
func TestProviderIgnoresCacheWithoutExperiment(t *testing.T) {
	t.Parallel()

	var calls atomic.Int32

	v := venv.MemVenv().WithHandler(func(_ context.Context, inv vexec.Invocation) vexec.Result {
		calls.Add(1)
		assert.NotContains(t, venv.ParseEnviron(inv.Env), externalcmd.EnvSchema)

		return credsResult(t, 60, "dev/**")
	})
	root := newRepo(t, v)
	ctx := externalcmd.ContextWithResponseCache(t.Context())

	for range 2 {
		fetchAccessKeyID(ctx, t, v, experiment.NewExperiments(), filepath.Join(root, "dev", "vpc"))
	}

	assert.Equal(t, int32(2), calls.Load())
}

// TestProviderIgnoresMalformedCacheWithoutExperiment pins that a cache object of the wrong shape does not fail the
// fetch while response caching is disabled.
func TestProviderIgnoresMalformedCacheWithoutExperiment(t *testing.T) {
	t.Parallel()

	v := venv.MemVenv().WithHandler(func(_ context.Context, _ vexec.Invocation) vexec.Result {
		return vexec.Result{Stdout: []byte(
			`{"awsCredentials": {"ACCESS_KEY_ID": "` + cacheAccessKeyID +
				`", "SECRET_ACCESS_KEY": "fake-secret"}, "cache": {"ttl": "900"}}`,
		)}
	})
	unit := filepath.Join(v.Platform.TempDir(), "dev", "vpc")

	assert.Equal(t, cacheAccessKeyID, fetchAccessKeyID(t.Context(), t, v, experiment.NewExperiments(), unit))
}

// TestProviderCachesResponseForMatchingUnits pins that a response with a positive ttl is reused for units its
// globs match, and that a unit outside the globs runs the command.
func TestProviderCachesResponseForMatchingUnits(t *testing.T) {
	t.Parallel()

	var calls atomic.Int32

	v := venv.MemVenv().
		WithEnv(map[string]string{externalcmd.EnvCacheKey: "inherited"}).
		WithHandler(func(_ context.Context, inv vexec.Invocation) vexec.Result {
			calls.Add(1)

			env := venv.ParseEnviron(inv.Env)
			assert.Equal(t, externalcmd.SchemaVersion, env[externalcmd.EnvSchema])
			assert.NotContains(t, env, externalcmd.EnvCacheKey)

			return credsResult(t, 60, "dev/**", "shared/*")
		})
	root := newRepo(t, v)
	exps := cacheExperiments(t)
	ctx := externalcmd.ContextWithResponseCache(t.Context())

	for _, unit := range []string{"dev/vpc", "dev/eu/db", "shared/dns"} {
		assert.Equal(t, cacheAccessKeyID, fetchAccessKeyID(ctx, t, v, exps, filepath.Join(root, unit)))
	}

	assert.Equal(t, int32(1), calls.Load())

	// The command lists globs that miss these units, so its response is used and not stored.
	for _, unit := range []string{"prod/vpc", "dev", "shared/dns/zone"} {
		for range 2 {
			fetchAccessKeyID(ctx, t, v, exps, filepath.Join(root, unit))
		}
	}

	assert.Equal(t, int32(7), calls.Load())
}

// TestProviderCacheIsScopedToRepository pins that a response is reused only for units in the repository it was
// cached for, even when a unit in another repository matches its globs.
func TestProviderCacheIsScopedToRepository(t *testing.T) {
	t.Parallel()

	var calls atomic.Int32

	v := venv.MemVenv().WithHandler(func(_ context.Context, _ vexec.Invocation) vexec.Result {
		calls.Add(1)

		return credsResult(t, 60, "dev/**")
	})
	exps := cacheExperiments(t)
	ctx := externalcmd.ContextWithResponseCache(t.Context())

	for _, repo := range []string{"a", "b"} {
		root := filepath.Join(v.Platform.TempDir(), repo)
		require.NoError(t, v.FS.MkdirAll(filepath.Join(root, ".git"), 0o755))

		for range 2 {
			fetchAccessKeyID(ctx, t, v, exps, filepath.Join(root, "dev", "vpc"))
		}
	}

	assert.Equal(t, int32(2), calls.Load())
}

// TestProviderCachedEnvsAreNotShared pins that units reusing one response each get their own environment map.
func TestProviderCachedEnvsAreNotShared(t *testing.T) {
	t.Parallel()

	v := venv.MemVenv().WithHandler(func(_ context.Context, _ vexec.Invocation) vexec.Result {
		return vexec.Result{Stdout: []byte(
			`{"envs": {"ARM_CLIENT_ID": "fake-client-id"}, "cache": {"root": "repo", "globs": ["**"], "ttl": 60}}`,
		)}
	})
	root := newRepo(t, v)
	exps := cacheExperiments(t)
	ctx := externalcmd.ContextWithResponseCache(t.Context())

	first, err := newCacheProvider(exps, filepath.Join(root, "a")).GetCredentials(ctx, logger.CreateLogger(), v)
	require.NoError(t, err)

	first.Envs["ARM_CLIENT_ID"] = "overwritten"

	second, err := newCacheProvider(exps, filepath.Join(root, "b")).GetCredentials(ctx, logger.CreateLogger(), v)
	require.NoError(t, err)
	assert.Equal(t, "fake-client-id", second.Envs["ARM_CLIENT_ID"])
}

// TestProviderCacheExpiresAfterTTL pins that a cached response is reused until its ttl passes.
func TestProviderCacheExpiresAfterTTL(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		var calls atomic.Int32

		v := venv.MemVenv().WithHandler(func(_ context.Context, _ vexec.Invocation) vexec.Result {
			calls.Add(1)

			return credsResult(t, 60, "dev/**")
		})
		root := newRepo(t, v)
		exps := cacheExperiments(t)
		ctx := externalcmd.ContextWithResponseCache(t.Context())
		unit := filepath.Join(root, "dev", "vpc")

		fetchAccessKeyID(ctx, t, v, exps, unit)
		synctest.Sleep(59 * time.Second)
		fetchAccessKeyID(ctx, t, v, exps, unit)
		assert.Equal(t, int32(1), calls.Load())

		synctest.Sleep(time.Second)
		fetchAccessKeyID(ctx, t, v, exps, unit)
		assert.Equal(t, int32(2), calls.Load())
	})
}

// TestProviderCacheTTLIsCapped pins that a response is reused for at most MaxCacheTTL, however long its ttl.
func TestProviderCacheTTLIsCapped(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		var calls atomic.Int32

		v := venv.MemVenv().WithHandler(func(_ context.Context, _ vexec.Invocation) vexec.Result {
			calls.Add(1)

			return credsResult(t, int(2*externalcmd.MaxCacheTTL/time.Second), "dev/**")
		})
		root := newRepo(t, v)
		exps := cacheExperiments(t)
		ctx := externalcmd.ContextWithResponseCache(t.Context())
		unit := filepath.Join(root, "dev", "vpc")

		fetchAccessKeyID(ctx, t, v, exps, unit)
		synctest.Sleep(externalcmd.MaxCacheTTL - time.Second)
		fetchAccessKeyID(ctx, t, v, exps, unit)
		assert.Equal(t, int32(1), calls.Load())

		synctest.Sleep(time.Second)
		fetchAccessKeyID(ctx, t, v, exps, unit)
		assert.Equal(t, int32(2), calls.Load())
	})
}

// TestProviderCachesResponseWithMaxCacheGlobs pins that a cache object listing exactly MaxCacheGlobs globs is
// stored.
func TestProviderCachesResponseWithMaxCacheGlobs(t *testing.T) {
	t.Parallel()

	var calls atomic.Int32

	v := venv.MemVenv().WithHandler(func(_ context.Context, _ vexec.Invocation) vexec.Result {
		calls.Add(1)

		return vexec.Result{Stdout: []byte(
			`{"awsCredentials": {"ACCESS_KEY_ID": "` + cacheAccessKeyID +
				`", "SECRET_ACCESS_KEY": "fake-secret"}, "cache": {"root": "repo", "ttl": 60, "globs": [` +
				manyGlobs(externalcmd.MaxCacheGlobs) + `]}}`,
		)}
	})
	root := newRepo(t, v)
	exps := cacheExperiments(t)
	ctx := externalcmd.ContextWithResponseCache(t.Context())

	for range 2 {
		assert.Equal(t, cacheAccessKeyID, fetchAccessKeyID(ctx, t, v, exps, filepath.Join(root, "dev", "vpc")))
	}

	assert.Equal(t, int32(1), calls.Load())
}

// TestProviderCacheStoresAtMostMaxCacheEntries pins that once the cache has MaxCacheEntries live responses, a new
// response is used uncached while the stored ones are still reused, and that expired responses free their slots.
func TestProviderCacheStoresAtMostMaxCacheEntries(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		var (
			calls atomic.Int32
			root  string
		)

		v := venv.MemVenv().WithHandler(func(_ context.Context, inv vexec.Invocation) vexec.Result {
			calls.Add(1)

			rel, err := filepath.Rel(root, inv.Dir)
			require.NoError(t, err)

			return credsResult(t, 60, filepath.ToSlash(rel))
		})
		root = newRepo(t, v)
		exps := cacheExperiments(t)
		ctx := externalcmd.ContextWithResponseCache(t.Context())

		unit := func(i int) string {
			return filepath.Join(root, "units", strconv.Itoa(i))
		}

		for i := range externalcmd.MaxCacheEntries {
			fetchAccessKeyID(ctx, t, v, exps, unit(i))
		}

		require.Equal(t, int32(externalcmd.MaxCacheEntries), calls.Load())

		overflow := unit(externalcmd.MaxCacheEntries)

		for range 2 {
			assert.Equal(t, cacheAccessKeyID, fetchAccessKeyID(ctx, t, v, exps, overflow))
		}

		fetchAccessKeyID(ctx, t, v, exps, unit(0))
		assert.Equal(t, int32(externalcmd.MaxCacheEntries+2), calls.Load())

		synctest.Sleep(time.Minute)

		for range 2 {
			fetchAccessKeyID(ctx, t, v, exps, overflow)
		}

		assert.Equal(t, int32(externalcmd.MaxCacheEntries+3), calls.Load())
	})
}

// TestProviderUsesUncachedResponse pins the responses whose credentials are used without being stored: a zero
// ttl, and a cache object that cannot be resolved to a key.
func TestProviderUsesUncachedResponse(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		cache string
	}{
		{name: "zero ttl", cache: `{"root": "repo", "globs": ["dev/**"], "ttl": 0}`},
		{name: "absent ttl", cache: `{"root": "repo", "globs": ["dev/**"]}`},
		{name: "unknown root", cache: `{"root": "workspace", "globs": ["dev/**"], "ttl": 60}`},
		{name: "empty globs", cache: `{"root": "repo", "globs": [], "ttl": 60}`},
		{name: "invalid glob", cache: `{"root": "repo", "globs": ["dev/[a-"], "ttl": 60}`},
		{name: "globs miss the unit", cache: `{"root": "repo", "globs": ["prod/**"], "ttl": 60}`},
		{name: "string ttl", cache: `{"root": "repo", "globs": ["dev/**"], "ttl": "900"}`},
		{name: "cache is not an object", cache: `"dev/**"`},
		{
			name:  "too many globs",
			cache: `{"root": "repo", "ttl": 60, "globs": [` + manyGlobs(externalcmd.MaxCacheGlobs+1) + `]}`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var calls atomic.Int32

			v := venv.MemVenv().WithHandler(func(_ context.Context, _ vexec.Invocation) vexec.Result {
				calls.Add(1)

				return vexec.Result{Stdout: []byte(
					`{"awsCredentials": {"ACCESS_KEY_ID": "` + cacheAccessKeyID +
						`", "SECRET_ACCESS_KEY": "fake-secret"}, "cache": ` + tc.cache + `}`,
				)}
			})
			root := newRepo(t, v)
			exps := cacheExperiments(t)
			ctx := externalcmd.ContextWithResponseCache(t.Context())

			for range 2 {
				unit := filepath.Join(root, "dev", "vpc")
				assert.Equal(t, cacheAccessKeyID, fetchAccessKeyID(ctx, t, v, exps, unit))
			}

			assert.Equal(t, int32(2), calls.Load())
		})
	}
}

// TestProviderUsesUncachedResponseOutsideRepository pins that a unit in no git repository, where the repo root
// cannot be resolved, gets the credentials of a response it cannot cache.
func TestProviderUsesUncachedResponseOutsideRepository(t *testing.T) {
	t.Parallel()

	var calls atomic.Int32

	v := venv.MemVenv().WithHandler(func(_ context.Context, _ vexec.Invocation) vexec.Result {
		calls.Add(1)

		return credsResult(t, 60, "**")
	})
	exps := cacheExperiments(t)
	ctx := externalcmd.ContextWithResponseCache(t.Context())
	unit := filepath.Join(v.Platform.TempDir(), "dev", "vpc")

	for range 2 {
		assert.Equal(t, cacheAccessKeyID, fetchAccessKeyID(ctx, t, v, exps, unit))
	}

	assert.Equal(t, int32(2), calls.Load())
}

// TestProviderPreflightIsFollowedByKeyedRequest pins the two-request exchange: the root and globs of a response
// with a negative ttl become the key, the next request receives it in the environment, and its response is
// stored under the preflight's globs.
func TestProviderPreflightIsFollowedByKeyedRequest(t *testing.T) {
	t.Parallel()

	var (
		preflights atomic.Int32
		keyed      atomic.Int32
		root       string
	)

	v := venv.MemVenv().WithHandler(func(_ context.Context, inv vexec.Invocation) vexec.Result {
		env := venv.ParseEnviron(inv.Env)
		assert.Equal(t, externalcmd.SchemaVersion, env[externalcmd.EnvSchema])

		key, ok := env[externalcmd.EnvCacheKey]
		if !ok {
			preflights.Add(1)

			return preflightResult(t, "dev/**")
		}

		keyed.Add(1)

		var got struct {
			Root  string   `json:"root"`
			Globs []string `json:"globs"`
		}

		require.NoError(t, json.Unmarshal([]byte(key), &got))
		assert.Equal(t, root, got.Root)
		assert.Equal(t, []string{"dev/**"}, got.Globs)

		// The globs of a keyed response are ignored in favor of the preflight's.
		return credsResult(t, 60, "prod/**")
	})
	root = newRepo(t, v)
	exps := cacheExperiments(t)
	ctx := externalcmd.ContextWithResponseCache(t.Context())

	for _, unit := range []string{"dev/vpc", "dev/db"} {
		assert.Equal(t, cacheAccessKeyID, fetchAccessKeyID(ctx, t, v, exps, filepath.Join(root, unit)))
	}

	assert.Equal(t, int32(1), preflights.Load())
	assert.Equal(t, int32(1), keyed.Load())
}

// TestProviderKeyedResponseExpiresAfterTTL pins that a response stored by a keyed request is reused until its ttl
// passes, after which the next unit starts the exchange again.
func TestProviderKeyedResponseExpiresAfterTTL(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		var preflights, keyed atomic.Int32

		v := venv.MemVenv().WithHandler(func(_ context.Context, inv vexec.Invocation) vexec.Result {
			if _, ok := venv.ParseEnviron(inv.Env)[externalcmd.EnvCacheKey]; !ok {
				preflights.Add(1)

				return preflightResult(t, "dev/**")
			}

			keyed.Add(1)

			return credsResult(t, 60, "dev/**")
		})
		root := newRepo(t, v)
		exps := cacheExperiments(t)
		ctx := externalcmd.ContextWithResponseCache(t.Context())

		fetchAccessKeyID(ctx, t, v, exps, filepath.Join(root, "dev", "vpc"))
		synctest.Sleep(59 * time.Second)
		fetchAccessKeyID(ctx, t, v, exps, filepath.Join(root, "dev", "db"))
		assert.Equal(t, int32(1), preflights.Load())
		assert.Equal(t, int32(1), keyed.Load())

		synctest.Sleep(time.Second)
		assert.Equal(t, cacheAccessKeyID, fetchAccessKeyID(ctx, t, v, exps, filepath.Join(root, "dev", "db")))
		assert.Equal(t, int32(2), preflights.Load())
		assert.Equal(t, int32(2), keyed.Load())
	})
}

// TestProviderConcurrentPreflightsShareKeyedRequestWithRacing pins that units preflighting to one key at the same
// time run a single keyed request between them.
func TestProviderConcurrentPreflightsShareKeyedRequestWithRacing(t *testing.T) {
	t.Parallel()

	const units = 16

	var (
		preflights atomic.Int32
		keyed      atomic.Int32
	)

	allPreflighted := make(chan struct{})

	v := venv.MemVenv().WithHandler(func(ctx context.Context, inv vexec.Invocation) vexec.Result {
		if _, ok := venv.ParseEnviron(inv.Env)[externalcmd.EnvCacheKey]; !ok {
			if preflights.Add(1) == units {
				close(allPreflighted)
			}

			return preflightResult(t, "dev/**")
		}

		keyed.Add(1)

		select {
		case <-allPreflighted:
		case <-ctx.Done():
			return vexec.Result{Err: ctx.Err()}
		}

		return credsResult(t, 60, "dev/**")
	})
	root := newRepo(t, v)
	exps := cacheExperiments(t)
	ctx := externalcmd.ContextWithResponseCache(t.Context())

	var g errgroup.Group

	for i := range units {
		g.Go(func() error {
			unit := filepath.Join(root, "dev", "unit-"+string(rune('a'+i)))
			_, err := newCacheProvider(exps, unit).GetCredentials(ctx, logger.CreateLogger(), v.WithEnvCloned())

			return err
		})
	}

	require.NoError(t, g.Wait())
	assert.Equal(t, int32(units), preflights.Load())
	assert.Equal(t, int32(1), keyed.Load())
}

// TestProviderFailedKeyedRequestFailsWaitersWithRacing pins that a keyed request that fails returns its error to
// every unit waiting on it, and is not cached, so the next unit runs the command again.
func TestProviderFailedKeyedRequestFailsWaitersWithRacing(t *testing.T) {
	t.Parallel()

	errAuthFailed := errors.New("fake auth failure")

	synctest.Test(t, func(t *testing.T) {
		var keyed atomic.Int32

		release := make(chan struct{})

		v := venv.MemVenv().WithHandler(func(_ context.Context, inv vexec.Invocation) vexec.Result {
			if _, ok := venv.ParseEnviron(inv.Env)[externalcmd.EnvCacheKey]; !ok {
				return preflightResult(t, "dev/**")
			}

			if keyed.Add(1) > 1 {
				return credsResult(t, 60, "dev/**")
			}

			<-release

			return vexec.Result{Err: errAuthFailed}
		})
		root := newRepo(t, v)
		exps := cacheExperiments(t)
		ctx := externalcmd.ContextWithResponseCache(t.Context())

		var leader, waiter errgroup.Group

		leader.Go(func() error {
			_, err := newCacheProvider(exps, filepath.Join(root, "dev", "a")).
				GetCredentials(ctx, logger.CreateLogger(), v.WithEnvCloned())

			return err
		})

		synctest.Wait()

		waiter.Go(func() error {
			_, err := newCacheProvider(exps, filepath.Join(root, "dev", "b")).
				GetCredentials(ctx, logger.CreateLogger(), v.WithEnvCloned())

			return err
		})

		synctest.Wait()
		close(release)

		require.ErrorIs(t, leader.Wait(), errAuthFailed)
		require.ErrorIs(t, waiter.Wait(), errAuthFailed)
		assert.Equal(t, int32(1), keyed.Load())

		assert.Equal(t, cacheAccessKeyID, fetchAccessKeyID(ctx, t, v, exps, filepath.Join(root, "dev", "c")))
		assert.Equal(t, int32(2), keyed.Load())
	})
}

// TestProviderKeyedRequestSurvivesCancelledLeaderWithRacing pins that a unit waiting on a keyed request runs it
// again when the unit that started the request is cancelled.
func TestProviderKeyedRequestSurvivesCancelledLeaderWithRacing(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		var keyed atomic.Int32

		v := venv.MemVenv().WithHandler(func(ctx context.Context, inv vexec.Invocation) vexec.Result {
			if _, ok := venv.ParseEnviron(inv.Env)[externalcmd.EnvCacheKey]; !ok {
				return preflightResult(t, "dev/**")
			}

			if keyed.Add(1) > 1 {
				return credsResult(t, 60, "dev/**")
			}

			<-ctx.Done()

			return vexec.Result{Err: ctx.Err()}
		})
		root := newRepo(t, v)
		exps := cacheExperiments(t)
		ctx := externalcmd.ContextWithResponseCache(t.Context())

		leaderCtx, cancelLeader := context.WithCancel(ctx)
		defer cancelLeader()

		var leader, waiter errgroup.Group

		leader.Go(func() error {
			_, err := newCacheProvider(exps, filepath.Join(root, "dev", "a")).
				GetCredentials(leaderCtx, logger.CreateLogger(), v.WithEnvCloned())

			return err
		})

		synctest.Wait()

		var waiterCreds *providers.Credentials

		waiter.Go(func() error {
			var err error

			waiterCreds, err = newCacheProvider(exps, filepath.Join(root, "dev", "b")).
				GetCredentials(ctx, logger.CreateLogger(), v.WithEnvCloned())

			return err
		})

		synctest.Wait()
		cancelLeader()

		require.ErrorIs(t, leader.Wait(), context.Canceled)
		require.NoError(t, waiter.Wait())
		assert.Equal(t, cacheAccessKeyID, waiterCreds.Envs["AWS_ACCESS_KEY_ID"])
		assert.Equal(t, int32(2), keyed.Load())
	})
}

// TestProviderPreflightWithUnusableKeyFails pins that a preflight whose cache object cannot be resolved to a key
// fails the fetch with the reason, as it has no credentials to use instead.
func TestProviderPreflightWithUnusableKeyFails(t *testing.T) {
	t.Parallel()

	tcs := []struct {
		wantErr error
		name    string
		cache   string
	}{
		{
			name:    "unknown root",
			cache:   `{"root": "workspace", "globs": ["dev/**"], "ttl": -1}`,
			wantErr: externalcmd.ErrCacheRootUnknown,
		},
		{
			name:    "empty globs",
			cache:   `{"root": "repo", "globs": [], "ttl": -1}`,
			wantErr: externalcmd.ErrCacheGlobsEmpty,
		},
		{
			name:    "too many globs",
			cache:   `{"root": "repo", "ttl": -1, "globs": [` + manyGlobs(externalcmd.MaxCacheGlobs+1) + `]}`,
			wantErr: externalcmd.ErrCacheGlobsTooMany,
		},
		{
			name:    "invalid glob",
			cache:   `{"root": "repo", "globs": ["dev/[a-"], "ttl": -1}`,
			wantErr: externalcmd.ErrCacheGlobInvalid,
		},
		{
			name:    "globs miss the unit",
			cache:   `{"root": "repo", "globs": ["prod/**"], "ttl": -1}`,
			wantErr: externalcmd.ErrCacheGlobsMissUnit,
		},
	}

	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			v := venv.MemVenv().WithHandler(func(_ context.Context, _ vexec.Invocation) vexec.Result {
				return vexec.Result{Stdout: []byte(`{"cache": ` + tc.cache + `}`)}
			})
			root := newRepo(t, v)
			ctx := externalcmd.ContextWithResponseCache(t.Context())

			_, err := newCacheProvider(cacheExperiments(t), filepath.Join(root, "dev", "vpc")).
				GetCredentials(ctx, logger.CreateLogger(), v)
			require.ErrorIs(t, err, externalcmd.ErrPreflightKeyUnusable)
			require.ErrorIs(t, err, tc.wantErr)
		})
	}
}

// TestProviderPreflightOutsideRepositoryFails pins that a preflight for a unit in no git repository fails the fetch,
// as its key cannot be scoped to a repository.
func TestProviderPreflightOutsideRepositoryFails(t *testing.T) {
	t.Parallel()

	v := venv.MemVenv().WithHandler(func(_ context.Context, _ vexec.Invocation) vexec.Result {
		return preflightResult(t, "dev/**")
	})
	ctx := externalcmd.ContextWithResponseCache(t.Context())

	_, err := newCacheProvider(cacheExperiments(t), filepath.Join(v.Platform.TempDir(), "dev", "vpc")).
		GetCredentials(ctx, logger.CreateLogger(), v)
	require.ErrorIs(t, err, externalcmd.ErrPreflightKeyUnusable)
	require.ErrorIs(t, err, git.ErrNoRepoRoot)
}

// TestProviderMalformedPreflightCacheFails pins that a cache object with a negative ttl that does not decode fails
// the fetch.
func TestProviderMalformedPreflightCacheFails(t *testing.T) {
	t.Parallel()

	tcs := []struct {
		name  string
		cache string
	}{
		{name: "globs not a list", cache: `{"root": "repo", "globs": "dev/**", "ttl": -1}`},
		{name: "root not a string", cache: `{"root": 1, "globs": ["dev/**"], "ttl": -1}`},
	}

	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			v := venv.MemVenv().WithHandler(func(_ context.Context, _ vexec.Invocation) vexec.Result {
				return vexec.Result{Stdout: []byte(`{"cache": ` + tc.cache + `}`)}
			})
			root := newRepo(t, v)
			ctx := externalcmd.ContextWithResponseCache(t.Context())

			_, err := newCacheProvider(cacheExperiments(t), filepath.Join(root, "dev", "vpc")).
				GetCredentials(ctx, logger.CreateLogger(), v)
			require.ErrorIs(t, err, externalcmd.ErrPreflightKeyUnusable)
		})
	}
}

// TestProviderSecondPreflightFails pins that a keyed request answered with another preflight fails the fetch.
func TestProviderSecondPreflightFails(t *testing.T) {
	t.Parallel()

	var calls atomic.Int32

	v := venv.MemVenv().WithHandler(func(_ context.Context, _ vexec.Invocation) vexec.Result {
		calls.Add(1)

		return preflightResult(t, "dev/**")
	})
	root := newRepo(t, v)
	ctx := externalcmd.ContextWithResponseCache(t.Context())

	_, err := newCacheProvider(cacheExperiments(t), filepath.Join(root, "dev", "vpc")).
		GetCredentials(ctx, logger.CreateLogger(), v)
	require.ErrorIs(t, err, externalcmd.ErrKeyedResponseIsPreflight)
	assert.Equal(t, int32(2), calls.Load())
}

// cacheExperiments returns experiments with response caching enabled.
func cacheExperiments(t *testing.T) experiment.Experiments {
	t.Helper()

	exps := experiment.NewExperiments()
	require.NoError(t, exps.EnableExperiment(experiment.AuthProviderCache))

	return exps
}

// newRepo creates a git repository root on the filesystem of v and returns its path.
func newRepo(t *testing.T, v *venv.Venv) string {
	t.Helper()

	root := v.Platform.TempDir()
	require.NoError(t, v.FS.MkdirAll(filepath.Join(root, ".git"), 0o755))

	return root
}

// newCacheProvider returns a provider that runs cacheCmd for the unit in unitDir.
func newCacheProvider(exps experiment.Experiments, unitDir string) providers.Provider {
	return externalcmd.NewProvider(
		logger.CreateLogger(),
		cacheCmd,
		shell.NewShellOptions(map[string]string{}).WithWorkingDir(unitDir).WithExperiments(exps),
	)
}

// fetchAccessKeyID fetches credentials for the unit in unitDir and returns their AWS access key ID.
func fetchAccessKeyID(
	ctx context.Context,
	t *testing.T,
	v *venv.Venv,
	exps experiment.Experiments,
	unitDir string,
) string {
	t.Helper()

	creds, err := newCacheProvider(exps, unitDir).GetCredentials(ctx, logger.CreateLogger(), v)
	require.NoError(t, err)
	require.NotNil(t, creds)

	return creds.Envs["AWS_ACCESS_KEY_ID"]
}

// credsResult is a response with AWS credentials and a cache object rooted at the repository.
func credsResult(t *testing.T, ttl int, globs ...string) vexec.Result {
	t.Helper()

	return vexec.Result{Stdout: []byte(
		`{"awsCredentials": {"ACCESS_KEY_ID": "` + cacheAccessKeyID +
			`", "SECRET_ACCESS_KEY": "fake-secret"}, "cache": ` + cacheObject(t, ttl, globs) + `}`,
	)}
}

// preflightResult is a response with only a cache object rooted at the repository and a negative ttl.
func preflightResult(t *testing.T, globs ...string) vexec.Result {
	t.Helper()

	return vexec.Result{Stdout: []byte(`{"cache": ` + cacheObject(t, -1, globs) + `}`)}
}

// cacheObject returns the JSON of a cache object rooted at the repository.
func cacheObject(t *testing.T, ttl int, globs []string) string {
	t.Helper()

	cache, err := json.Marshal(map[string]any{"root": "repo", "ttl": ttl, "globs": globs})
	require.NoError(t, err)

	return string(cache)
}

// manyGlobs returns n `dev/**` globs as the elements of a JSON array, without the brackets.
func manyGlobs(n int) string {
	return strings.TrimSuffix(strings.Repeat(`"dev/**",`, n), ",")
}
