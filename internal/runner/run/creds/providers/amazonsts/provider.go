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

var awsCredentialEnvKeys = []string{
	"AWS_ACCESS_KEY_ID",
	"AWS_SECRET_ACCESS_KEY",
	"AWS_SESSION_TOKEN",
	"AWS_SECURITY_TOKEN",
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

	iamRoleOpts.AssumeRoleDuration = effectiveAssumeRoleDuration(iamRoleOpts.AssumeRoleDuration)
	roleKey := roleConfigKey(iamRoleOpts)
	sourceFP := sourceCredentialFingerprint(v.Env)
	identityKey := roleKey + "\x00" + sourceFP

	if entry := credentialsStore.get(identityKey); entry != nil {
		l.Debugf("Using cached credentials for IAM role %s.", iamRoleOpts.RoleARN)
		return entry.creds, nil
	}

	if accessKeyID := v.Env["AWS_ACCESS_KEY_ID"]; accessKeyID != "" {
		if entry := credentialsStore.getBySession(roleKey, accessKeyID); entry != nil {
			l.Debugf("Using cached credentials for IAM role %s.", iamRoleOpts.RoleARN)
			return entry.creds, nil
		}

		if expired := credentialsStore.getExpiredBySession(roleKey, accessKeyID); expired != nil {
			return provider.assumeAndCache(ctx, l, v, iamRoleOpts, roleKey, expired.sourceEnv)
		}
	}

	return provider.assumeAndCache(ctx, l, v, iamRoleOpts, roleKey, snapshotAWSCredentialEnv(v.Env))
}

func (provider *Provider) assumeAndCache(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	iamRoleOpts iam.RoleOptions,
	roleKey string,
	sourceEnv map[string]string,
) (*providers.Credentials, error) {
	identityKey := roleKey + "\x00" + sourceCredentialFingerprint(sourceEnv)

	vAny, err, _ := credentialsStore.flight.Do(identityKey, func() (any, error) {
		if entry := credentialsStore.get(identityKey); entry != nil {
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

		credentialsStore.put(identityKey, roleKey, &cacheEntry{
			creds:     creds,
			sourceEnv: maps.Clone(sourceEnv),
			expires:   cacheExpiration(resp, iamRoleOpts.AssumeRoleDuration),
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

func sourceCredentialFingerprint(env map[string]string) string {
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

func cacheExpiration(resp *types.Credentials, durationSecs int64) time.Time {
	expiresAt := time.Now().Add(time.Duration(durationSecs) * time.Second)
	if resp != nil && resp.Expiration != nil {
		expiresAt = aws.ToTime(resp.Expiration)
	}

	withWindow := expiresAt.Add(-credentialsCacheExpiryWindow)
	if withWindow.After(time.Now()) {
		return withWindow
	}

	return expiresAt
}

type cacheEntry struct {
	creds     *providers.Credentials
	sourceEnv map[string]string
	expires   time.Time
}

type stsCredentialsStore struct {
	flight    singleflight.Group
	byID      map[string]*cacheEntry
	bySession map[string]*cacheEntry
	mu        sync.Mutex
}

func (s *stsCredentialsStore) get(identityKey string) *cacheEntry {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.live(s.byID[identityKey])
}

func (s *stsCredentialsStore) getBySession(roleKey, accessKeyID string) *cacheEntry {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.live(s.bySession[sessionIndexKey(roleKey, accessKeyID)])
}

func (s *stsCredentialsStore) getExpiredBySession(roleKey, accessKeyID string) *cacheEntry {
	s.mu.Lock()
	defer s.mu.Unlock()

	entry := s.bySession[sessionIndexKey(roleKey, accessKeyID)]
	if entry == nil || !time.Now().After(entry.expires) {
		return nil
	}

	return entry
}

func (s *stsCredentialsStore) put(identityKey, roleKey string, entry *cacheEntry) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.byID[identityKey] = entry
	if accessKeyID := entry.creds.Envs["AWS_ACCESS_KEY_ID"]; accessKeyID != "" {
		s.bySession[sessionIndexKey(roleKey, accessKeyID)] = entry
	}
}

func (s *stsCredentialsStore) live(entry *cacheEntry) *cacheEntry {
	if entry == nil {
		return nil
	}

	if time.Now().After(entry.expires) {
		return nil
	}

	return entry
}

func sessionIndexKey(roleKey, accessKeyID string) string {
	return roleKey + "\x00" + accessKeyID
}

// credentialsStore caches assumed sessions for the process. Entries are keyed by
// role configuration and source-credential fingerprint, with a session-access-key
// index so a later call whose env already holds the assumed session still hits.
var credentialsStore = &stsCredentialsStore{
	byID:      make(map[string]*cacheEntry),
	bySession: make(map[string]*cacheEntry),
}
