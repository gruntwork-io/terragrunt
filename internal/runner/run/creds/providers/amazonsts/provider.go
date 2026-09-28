// Package amazonsts provides a credentials provider that obtains credentials by making API requests to Amazon STS.
package amazonsts

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"hash"
	"maps"
	"strconv"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sts/types"
	"github.com/gruntwork-io/terragrunt/internal/awshelper"
	"github.com/gruntwork-io/terragrunt/internal/iam"
	"github.com/gruntwork-io/terragrunt/internal/runner/run/creds/providers"
	"github.com/gruntwork-io/terragrunt/internal/telemetry"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"golang.org/x/sync/singleflight"
)

// credentialsCacheExpiryWindow reserves five minutes before STS credentials expire.
const credentialsCacheExpiryWindow = 5 * time.Minute

const credentialsCacheName = "credentialsCache"

var awsCredentialEnvKeys = []string{
	"AWS_ACCESS_KEY_ID",
	"AWS_SECRET_ACCESS_KEY",
	"AWS_SESSION_TOKEN",
	"AWS_SECURITY_TOKEN",
}

type credentialsStoreKey struct{}

// WithIsolatedCredentialsCache returns a context that carries a fresh STS
// credentials cache, so concurrent or repeated tests do not share process state.
func WithIsolatedCredentialsCache(ctx context.Context) context.Context {
	return context.WithValue(ctx, credentialsStoreKey{}, newCredentialsStore())
}

// Provider obtains credentials by making API requests to Amazon STS.
type Provider struct {
	env         map[string]string
	iamRoleOpts iam.RoleOptions
}

// NewProvider returns a new Provider instance.
func NewProvider(
	l log.Logger,
	iamRoleOpts iam.RoleOptions,
	env map[string]string,
) providers.Provider {
	return &Provider{
		iamRoleOpts: iamRoleOpts,
		env:         env,
	}
}

// Name implements providers.Name
func (provider *Provider) Name() string {
	return "API calls to Amazon STS"
}

// GetCredentials implements providers.GetCredentials. The STS call rides v's
// HTTP client.
func (provider *Provider) GetCredentials(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
) (*providers.Credentials, error) {
	iamRoleOpts := provider.iamRoleOpts
	if iamRoleOpts.RoleARN == "" {
		return nil, nil
	}

	store := storeFromContext(ctx)
	iamRoleOpts.AssumeRoleDuration = effectiveAssumeRoleDuration(iamRoleOpts.AssumeRoleDuration)
	roleKey := roleConfigKey(iamRoleOpts)
	sourceFP := credentialFingerprint(v.Env)
	identityKey := roleKey + "\x00" + sourceFP

	if entry := store.get(ctx, identityKey); entry != nil {
		l.Debugf("Using cached credentials for IAM role %s.", iamRoleOpts.RoleARN)
		return entry.creds, nil
	}

	// The same fingerprint identifies the caller either way: when the env still holds
	// the source credentials it keys the identity, and when a previous assumption has
	// written its session there it keys that session back to the entry that made it.
	if v.Env["AWS_ACCESS_KEY_ID"] != "" {
		if entry := store.getBySession(ctx, roleKey, sourceFP); entry != nil {
			l.Debugf("Using cached credentials for IAM role %s.", iamRoleOpts.RoleARN)
			return entry.creds, nil
		}

		if pastRefresh := store.getPastRefreshBySession(ctx, roleKey, sourceFP); pastRefresh != nil {
			creds, err := provider.assumeAndCache(ctx, l, v, store, iamRoleOpts, roleKey, pastRefresh.sourceEnv)
			if err == nil {
				return creds, nil
			}

			if time.Now().Before(pastRefresh.expiresAt) {
				l.Warnf(
					"Failed to refresh IAM role %s; reusing cached session until %s: %v",
					iamRoleOpts.RoleARN,
					pastRefresh.expiresAt.UTC().Format(time.RFC3339),
					err,
				)

				return pastRefresh.creds, nil
			}

			return nil, err
		}
	}

	return provider.assumeAndCache(ctx, l, v, store, iamRoleOpts, roleKey, store.sourceEnvFor(v.Env))
}

func (provider *Provider) assumeAndCache(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	store *stsCredentialsStore,
	iamRoleOpts iam.RoleOptions,
	roleKey string,
	sourceEnv map[string]string,
) (*providers.Credentials, error) {
	identityKey := roleKey + "\x00" + credentialFingerprint(sourceEnv)

	vAny, err, _ := store.flight.Do(identityKey, func() (any, error) {
		if entry := store.get(ctx, identityKey); entry != nil {
			return entry.creds, nil
		}

		l.Debugf("Assuming IAM role %s with a session duration of %d seconds.",
			iamRoleOpts.RoleARN, iamRoleOpts.AssumeRoleDuration)

		var resp *types.Credentials

		collectErr := telemetry.TelemeterFromContext(ctx).Collect(ctx, l, "creds_assume_role", map[string]any{
			"role_arn":     iamRoleOpts.RoleARN,
			"session_name": iamRoleOpts.AssumeRoleSessionName,
			"duration":     iamRoleOpts.AssumeRoleDuration,
		}, func(ctx context.Context, l log.Logger) error {
			var assumeErr error

			resp, assumeErr = awshelper.AssumeIamRole(ctx, assumeVenv(v, sourceEnv), iamRoleOpts, "")

			return assumeErr
		})
		if collectErr != nil {
			return nil, collectErr
		}

		creds := &providers.Credentials{
			Name: providers.AWSCredentials,
			Envs: map[string]string{
				"AWS_ACCESS_KEY_ID":     aws.ToString(resp.AccessKeyId),
				"AWS_SECRET_ACCESS_KEY": aws.ToString(resp.SecretAccessKey),
				"AWS_SESSION_TOKEN":     aws.ToString(resp.SessionToken),
				"AWS_SECURITY_TOKEN":    aws.ToString(resp.SessionToken),
			},
		}

		refreshAt, expiresAt := cacheTimes(resp, iamRoleOpts.AssumeRoleDuration)
		store.put(ctx, identityKey, roleKey, &cacheEntry{
			creds:     creds,
			sourceEnv: maps.Clone(sourceEnv),
			sessionFP: credentialFingerprint(creds.Envs),
			refreshAt: refreshAt,
			expiresAt: expiresAt,
		})

		return creds, nil
	})
	if err != nil {
		return nil, err
	}

	creds, ok := vAny.(*providers.Credentials)
	if !ok {
		return nil, fmt.Errorf("unexpected STS credentials cache result type %T", vAny)
	}

	return creds, nil
}

func effectiveAssumeRoleDuration(duration int64) int64 {
	if duration <= 0 {
		return iam.DefaultAssumeRoleDuration
	}

	return duration
}

func roleConfigKey(opts iam.RoleOptions) string {
	sum := sha256.New()
	writeFingerprintField(sum, opts.RoleARN)
	writeFingerprintField(sum, strconv.FormatInt(opts.AssumeRoleDuration, 10))
	writeFingerprintField(sum, opts.AssumeRoleSessionName)
	writeFingerprintField(sum, opts.WebIdentityToken)

	return hex.EncodeToString(sum.Sum(nil))
}

func credentialFingerprint(env map[string]string) string {
	sum := sha256.New()
	for _, key := range awsCredentialEnvKeys {
		writeFingerprintField(sum, env[key])
	}

	return hex.EncodeToString(sum.Sum(nil))
}

func writeFingerprintField(sum hash.Hash, value string) {
	_, _ = sum.Write([]byte(value))
	_, _ = sum.Write([]byte{0})
}

func snapshotAWSCredentialEnv(env map[string]string) map[string]string {
	out := make(map[string]string, len(awsCredentialEnvKeys))
	for _, key := range awsCredentialEnvKeys {
		if value := env[key]; value != "" {
			out[key] = value
		}
	}

	return out
}

func assumeVenv(v *venv.Venv, sourceEnv map[string]string) *venv.Venv {
	env := maps.Clone(v.Env)
	for _, key := range awsCredentialEnvKeys {
		delete(env, key)
	}

	maps.Copy(env, sourceEnv)

	return v.WithEnv(env)
}

func cacheTimes(resp *types.Credentials, durationSecs int64) (refreshAt, expiresAt time.Time) {
	expiresAt = time.Now().Add(time.Duration(durationSecs) * time.Second)
	if resp != nil && resp.Expiration != nil {
		expiresAt = aws.ToTime(resp.Expiration)
	}

	refreshAt = expiresAt.Add(-credentialsCacheExpiryWindow)
	if !refreshAt.After(time.Now()) {
		refreshAt = expiresAt
	}

	return refreshAt, expiresAt
}

type cacheEntry struct {
	creds     *providers.Credentials
	sourceEnv map[string]string
	refreshAt time.Time
	expiresAt time.Time
	sessionFP string
}

type stsCredentialsStore struct {
	byID        map[string]*cacheEntry
	bySession   map[string]*cacheEntry
	mintedBySFP map[string]*cacheEntry
	flight      *singleflight.Group
	name        string
	mu          sync.Mutex
}

func newCredentialsStore() *stsCredentialsStore {
	return &stsCredentialsStore{
		byID:        make(map[string]*cacheEntry),
		bySession:   make(map[string]*cacheEntry),
		mintedBySFP: make(map[string]*cacheEntry),
		flight:      &singleflight.Group{},
		name:        credentialsCacheName,
	}
}

// sourceEnvFor returns the identity that should sign an assume-role request made from env.
func (s *stsCredentialsStore) sourceEnvFor(env map[string]string) map[string]string {
	snapshot := snapshotAWSCredentialEnv(env)
	if len(snapshot) == 0 {
		return snapshot
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// Each step consumes one minted session, so the chain cannot be longer than the index.
	for range len(s.mintedBySFP) {
		entry, minted := s.mintedBySFP[credentialFingerprint(snapshot)]
		if !minted || entry.sourceEnv == nil {
			return snapshot
		}

		snapshot = entry.sourceEnv
	}

	return snapshot
}

func storeFromContext(ctx context.Context) *stsCredentialsStore {
	if store, ok := ctx.Value(credentialsStoreKey{}).(*stsCredentialsStore); ok && store != nil {
		return store
	}

	return globalCredentialsStore
}

func (s *stsCredentialsStore) get(ctx context.Context, identityKey string) *cacheEntry {
	s.mu.Lock()
	defer s.mu.Unlock()

	telemetry.TelemeterFromContext(ctx).Count(ctx, s.name+"_cache_get", 1)

	entry, found := s.byID[identityKey]
	if !found {
		telemetry.TelemeterFromContext(ctx).Count(ctx, s.name+"_cache_miss", 1)
		return nil
	}

	if time.Now().After(entry.refreshAt) {
		telemetry.TelemeterFromContext(ctx).Count(ctx, s.name+"_cache_expiry", 1)
		return nil
	}

	telemetry.TelemeterFromContext(ctx).Count(ctx, s.name+"_cache_hit", 1)

	return entry
}

func (s *stsCredentialsStore) getBySession(ctx context.Context, roleKey, sessionFP string) *cacheEntry {
	s.mu.Lock()
	defer s.mu.Unlock()

	telemetry.TelemeterFromContext(ctx).Count(ctx, s.name+"_cache_get", 1)

	entry, found := s.bySession[sessionIndexKey(roleKey, sessionFP)]
	if !found {
		telemetry.TelemeterFromContext(ctx).Count(ctx, s.name+"_cache_miss", 1)
		return nil
	}

	if time.Now().After(entry.refreshAt) {
		telemetry.TelemeterFromContext(ctx).Count(ctx, s.name+"_cache_expiry", 1)
		return nil
	}

	telemetry.TelemeterFromContext(ctx).Count(ctx, s.name+"_cache_hit", 1)

	return entry
}

func (s *stsCredentialsStore) getPastRefreshBySession(ctx context.Context, roleKey, sessionFP string) *cacheEntry {
	s.mu.Lock()
	defer s.mu.Unlock()

	telemetry.TelemeterFromContext(ctx).Count(ctx, s.name+"_cache_get", 1)

	key := sessionIndexKey(roleKey, sessionFP)

	entry, found := s.bySession[key]
	if !found {
		telemetry.TelemeterFromContext(ctx).Count(ctx, s.name+"_cache_miss", 1)
		return nil
	}

	now := time.Now()
	if !now.After(entry.refreshAt) {
		telemetry.TelemeterFromContext(ctx).Count(ctx, s.name+"_cache_hit", 1)
		return nil
	}

	telemetry.TelemeterFromContext(ctx).Count(ctx, s.name+"_cache_expiry", 1)

	if now.After(entry.expiresAt) {
		delete(s.bySession, key)
		s.deleteIdentityLocked(entry)

		return nil
	}

	return entry
}

func (s *stsCredentialsStore) put(ctx context.Context, identityKey, roleKey string, entry *cacheEntry) {
	s.mu.Lock()
	defer s.mu.Unlock()

	telemetry.TelemeterFromContext(ctx).Count(ctx, s.name+"_cache_put", 1)

	previous := s.byID[identityKey]
	if previous != nil && previous.sessionFP != "" && previous.sessionFP != entry.sessionFP {
		delete(s.bySession, sessionIndexKey(roleKey, previous.sessionFP))
	}

	s.byID[identityKey] = entry
	if entry.sessionFP != "" {
		s.bySession[sessionIndexKey(roleKey, entry.sessionFP)] = entry
		// Indexed by session alone so any minted session is recognized whatever the role options were.
		s.mintedBySFP[entry.sessionFP] = entry
	}
}

func (s *stsCredentialsStore) deleteIdentityLocked(entry *cacheEntry) {
	for key, candidate := range s.byID {
		if candidate == entry {
			delete(s.byID, key)
			return
		}
	}
}

func sessionIndexKey(roleKey, sessionFP string) string {
	return roleKey + "\x00" + sessionFP
}

// globalCredentialsStore caches assumed sessions for the process.
var globalCredentialsStore = newCredentialsStore()
