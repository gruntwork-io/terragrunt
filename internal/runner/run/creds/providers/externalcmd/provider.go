// Package externalcmd provides a provider that runs an external command that returns a json string with credentials.
package externalcmd

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"strings"

	"github.com/gruntwork-io/terragrunt/internal/experiment"
	"github.com/gruntwork-io/terragrunt/internal/iam"
	"github.com/gruntwork-io/terragrunt/internal/runner/run/creds/providers"
	"github.com/gruntwork-io/terragrunt/internal/runner/run/creds/providers/amazonsts"
	"github.com/gruntwork-io/terragrunt/internal/shell"
	"github.com/gruntwork-io/terragrunt/internal/shell/split"
	"github.com/gruntwork-io/terragrunt/internal/telemetry"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/pkg/log"
)

// Provider runs external command that returns a json string with credentials.
type Provider struct {
	runOpts         *shell.ShellOptions
	authProviderCmd string
}

// NewProvider returns a new Provider instance.
func NewProvider(
	l log.Logger,
	authProviderCmd string,
	runOpts *shell.ShellOptions,
) providers.Provider {
	return &Provider{
		authProviderCmd: authProviderCmd,
		runOpts:         runOpts,
	}
}

// Name implements providers.Name
func (provider *Provider) Name() string {
	return fmt.Sprintf("external %s command", provider.authProviderCmd)
}

// GetCredentials implements providers.GetCredentials. When no auth provider command is
// configured the call is a no-op short-circuit; we skip emitting the obtain_creds span
// in that case so the trace isn't polluted with zero-duration spans.
//
// Panics when response caching is enabled and ctx has no [ResponseCache] from [ContextWithResponseCache].
func (provider *Provider) GetCredentials(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
) (*providers.Credentials, error) {
	if provider.authProviderCmd == "" {
		return nil, nil
	}

	var creds *providers.Credentials

	err := telemetry.TelemeterFromContext(ctx).Collect(ctx, l, "obtain_creds", map[string]any{
		"auth_provider_cmd": provider.authProviderCmd,
		"provider":          "external_cmd",
	}, func(credsCtx context.Context, l log.Logger) error {
		var fetchErr error

		creds, fetchErr = provider.fetchCredentials(credsCtx, l, v)

		return fetchErr
	})

	return creds, err
}

// fetchCredentials obtains the response of the configured auth-provider command and derives
// [providers.Credentials] from it. Callers go through [Provider.GetCredentials], which adds the obtain_creds
// telemetry span around this work.
//
// Panics when response caching is enabled and ctx has no [ResponseCache] from [ContextWithResponseCache].
func (provider *Provider) fetchCredentials(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
) (*providers.Credentials, error) {
	resp, err := provider.response(ctx, l, v)
	if err != nil {
		return nil, err
	}

	return provider.credentials(ctx, l, v, resp), nil
}

// response returns the auth-provider command response for the unit in the working directory. It runs the command
// unless the cache has a response for the unit.
//
// Panics when response caching is enabled and ctx has no [ResponseCache] from [ContextWithResponseCache].
func (provider *Provider) response(ctx context.Context, l log.Logger, v *venv.Venv) (*Response, error) {
	if !provider.runOpts.Experiments.Evaluate(experiment.AuthProviderCache) {
		return provider.runCommand(ctx, l, v)
	}

	return provider.cachedResponse(ctx, l, v)
}

// cachedResponse returns a cached response whose globs match the unit, or runs the command and applies the cache
// object of its response: a positive ttl stores the response, and a negative ttl makes it a preflight that is
// followed by a keyed request.
//
// Returns [ErrPreflightKeyUnusable] when a preflight has a cache object that cannot be resolved to a key, and
// [ErrKeyedResponseIsPreflight] when the request that follows a preflight is answered with another preflight.
//
// Panics when ctx has no [ResponseCache] from [ContextWithResponseCache].
func (provider *Provider) cachedResponse(ctx context.Context, l log.Logger, v *venv.Venv) (*Response, error) {
	responses := responseCacheFromContext(ctx)

	unit, err := resolveUnitPath(ctx, v, provider.runOpts.WorkingDir)
	if err != nil {
		return provider.unscopedResponse(ctx, l, v, err)
	}

	if resp, ok := responses.lookup(unit); ok {
		l.Debugf("Reusing the cached response of the %s.", provider.Name())

		return resp, nil
	}

	resp, err := provider.runCommand(ctx, l, requestVenv(v, ""))
	if err != nil {
		return nil, err
	}

	if resp.Cache == nil || resp.Cache.TTL == 0 {
		return resp, nil
	}

	key, err := responses.newKey(l, resp.Cache, unit)
	if err != nil {
		return provider.uncachedResponse(l, resp, err)
	}

	if resp.Cache.TTL < 0 {
		return provider.keyedResponse(ctx, l, v, responses, key)
	}

	provider.store(l, responses, key, resp)

	return resp, nil
}

// unscopedResponse runs the command for a unit whose repository could not be resolved, so its response cannot be
// cached.
//
// Returns unitErr, wrapped in [ErrPreflightKeyUnusable], when the response is a preflight.
func (provider *Provider) unscopedResponse(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	unitErr error,
) (*Response, error) {
	resp, err := provider.runCommand(ctx, l, requestVenv(v, ""))
	if err != nil {
		return nil, err
	}

	if resp.Cache == nil || resp.Cache.TTL == 0 {
		return resp, nil
	}

	return provider.uncachedResponse(l, resp, fmt.Errorf("resolving the repository of the unit: %w", unitErr))
}

// uncachedResponse returns resp for use without storing it, as err kept its cache object from resolving to a key.
//
// Returns err, wrapped in [ErrPreflightKeyUnusable], when resp is a preflight, which has no credentials to use.
func (provider *Provider) uncachedResponse(l log.Logger, resp *Response, err error) (*Response, error) {
	if resp.Cache.TTL < 0 {
		return nil, fmt.Errorf("%w: %w", ErrPreflightKeyUnusable, err)
	}

	l.Warnf("Not caching the response of the %s: %v", provider.Name(), err)

	return resp, nil
}

// keyedResponse runs the request that follows a preflight, with key in the command environment. Concurrent callers
// with the same key share one run of the command. The response is stored under key, and its own root and globs
// are ignored.
//
// Returns [ErrKeyedResponseIsPreflight] when the command answers with another preflight.
func (provider *Provider) keyedResponse(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	responses *ResponseCache,
	key cacheKey,
) (*Response, error) {
	return responses.flight(ctx, key, func() (*Response, error) {
		if resp, ok := responses.get(key); ok {
			return resp, nil
		}

		resp, err := provider.runCommand(ctx, l, requestVenv(v, key.id))
		if err != nil {
			return nil, err
		}

		if resp.Cache == nil || resp.Cache.TTL == 0 {
			return resp, nil
		}

		if resp.Cache.TTL < 0 {
			return nil, ErrKeyedResponseIsPreflight
		}

		provider.store(l, responses, key, resp)

		return resp, nil
	})
}

// store caches resp under key for the ttl of its cache object, shortened to [MaxCacheTTL].
func (provider *Provider) store(l log.Logger, responses *ResponseCache, key cacheKey, resp *Response) {
	ttl := min(resp.Cache.TTL, maxCacheTTLSeconds)

	if !responses.put(key, resp, ttl) {
		l.Warnf("Not caching the response of the %s: the cache already has %d responses.",
			provider.Name(), MaxCacheEntries)

		return
	}

	if ttl < resp.Cache.TTL {
		l.Warnf("Caching the response of the %s for %d seconds, the limit, instead of its ttl of %d seconds.",
			provider.Name(), ttl, resp.Cache.TTL)
	}
}

// runCommand runs the configured auth-provider command with the environment of v and decodes its JSON response.
//
// Returns [ErrPreflightKeyUnusable] when the cache object has a negative ttl and does not decode.
func (provider *Provider) runCommand(ctx context.Context, l log.Logger, v *venv.Venv) (*Response, error) {
	parts, err := split.Command(provider.authProviderCmd)
	if err != nil {
		return nil, fmt.Errorf("failed to parse auth provider command: %w", err)
	}

	if len(parts) == 0 {
		return nil, ErrEmptyAuthProviderCmd
	}

	command := parts[0]

	args := []string{}
	if len(parts) > 1 {
		args = parts[1:]
	}

	output, err := shell.RunCommandWithOutput(
		ctx, l, v, provider.runOpts,
		"", true, false, command, args...,
	)
	if err != nil {
		return nil, err
	}

	if output.Stdout.String() == "" {
		return nil, fmt.Errorf(
			"command %s completed successfully, but the response does not contain JSON string",
			provider.authProviderCmd)
	}

	var raw rawResponse

	if err := json.Unmarshal(output.Stdout.Bytes(), &raw); err != nil {
		return nil, fmt.Errorf("command %s returned a response with invalid JSON format", command)
	}

	resp := &raw.Response

	if len(raw.Cache) == 0 || !provider.runOpts.Experiments.Evaluate(experiment.AuthProviderCache) {
		return resp, nil
	}

	var cache *Cache

	if err := json.Unmarshal(raw.Cache, &cache); err != nil {
		if cache != nil && cache.TTL < 0 {
			return nil, fmt.Errorf("%w: invalid cache object: %w", ErrPreflightKeyUnusable, err)
		}

		l.Warnf("Not caching the response of the %s: invalid cache object: %v", provider.Name(), err)

		return resp, nil
	}

	resp.Cache = cache

	return resp, nil
}

// credentials derives [providers.Credentials] from resp, assuming the role of an awsRole response through STS.
func (provider *Provider) credentials(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	resp *Response,
) *providers.Credentials {
	creds := &providers.Credentials{
		Name: providers.AWSCredentials,
		Envs: make(map[string]string, len(resp.Envs)),
	}

	maps.Copy(creds.Envs, resp.Envs)

	if resp.AWSCredentials != nil {
		if envs := resp.AWSCredentials.Envs(ctx, l, provider.authProviderCmd); envs != nil {
			l.Debugf("Obtaining AWS credentials from the %s.", provider.Name())
			maps.Copy(creds.Envs, envs)
		}

		return creds
	}

	if resp.AWSRole != nil {
		if envs := resp.AWSRole.Envs(ctx, l, v, provider.authProviderCmd); envs != nil {
			l.Debugf("Assuming AWS role %s using the %s.", resp.AWSRole.RoleARN, provider.Name())
			maps.Copy(creds.Envs, envs)
		}

		return creds
	}

	return creds
}

// Response is the JSON response expected from an auth provider command.
type Response struct {
	// AWSCredentials contains AWS credentials to set as environment variables.
	AWSCredentials *AWSCredentials `json:"awsCredentials,omitempty"`
	// AWSRole contains AWS role information for role assumption.
	AWSRole *AWSRole `json:"awsRole,omitempty"`
	// Envs contains additional environment variables to set.
	Envs map[string]string `json:"envs,omitempty"`
	// Cache lets Terragrunt reuse the response for other units.
	Cache *Cache `json:"cache,omitempty"`
}

// rawResponse is a [Response] whose cache object is decoded on its own, so a malformed one cannot discard the
// credentials.
type rawResponse struct {
	Response
	Cache json.RawMessage `json:"cache"`
}

// AWSCredentials is the JSON schema for direct AWS credentials.
type AWSCredentials struct {
	// AccessKeyID is the AWS access key ID.
	AccessKeyID string `json:"ACCESS_KEY_ID" jsonschema:"required"`
	// SecretAccessKey is the AWS secret access key.
	SecretAccessKey string `json:"SECRET_ACCESS_KEY" jsonschema:"required"`
	// SessionToken is the AWS session token (optional).
	SessionToken string `json:"SESSION_TOKEN,omitempty"`
}

// AWSRole is the JSON schema for AWS role assumption.
type AWSRole struct {
	// RoleARN is the ARN of the IAM role to assume.
	RoleARN string `json:"roleARN" jsonschema:"required"`
	// RoleSessionName is the session name for the assumed role.
	RoleSessionName string `json:"roleSessionName,omitempty"`
	// WebIdentityToken is the web identity token for OIDC-based role assumption.
	WebIdentityToken string `json:"webIdentityToken,omitempty"`
	// Duration is the duration in seconds for the assumed role session.
	Duration int64 `json:"duration,omitempty" jsonschema:"minimum=0"`
}

// Envs assumes the role through STS and returns the AWS credential environment variables of the new session.
// It logs a warning and returns nil when roleARN is empty or the role cannot be assumed.
func (role *AWSRole) Envs(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	authProviderCmd string,
) map[string]string {
	if role.RoleARN == "" {
		l.Warnf("The command %s completed successfully, but AWS role assumption"+
			" contains empty required value: roleARN, nothing is being done.", authProviderCmd)

		return nil
	}

	// Left empty for AssumeIamRole to fill at call time; a per-fetch name would break the STS cache key.
	sessionName := role.RoleSessionName

	duration := role.Duration
	if duration == 0 {
		duration = iam.DefaultAssumeRoleDuration
	}

	iamRoleOpts := iam.RoleOptions{
		RoleARN:               role.RoleARN,
		AssumeRoleDuration:    duration,
		AssumeRoleSessionName: sessionName,
	}

	if role.WebIdentityToken != "" {
		iamRoleOpts.WebIdentityToken = role.WebIdentityToken
	}

	provider := amazonsts.NewProvider(l, iamRoleOpts, nil)

	creds, err := provider.GetCredentials(ctx, l, v)
	if err != nil {
		l.Warnf("Failed to assume role %s: %v", role.RoleARN, err)
		return nil
	}

	if creds == nil {
		l.Warnf("The command %s completed successfully,"+
			" but failed to assume role %s, nothing is being done.",
			authProviderCmd, role.RoleARN)

		return nil
	}

	envs := map[string]string{
		"AWS_ACCESS_KEY_ID":     creds.Envs["AWS_ACCESS_KEY_ID"],
		"AWS_SECRET_ACCESS_KEY": creds.Envs["AWS_SECRET_ACCESS_KEY"],
		"AWS_SESSION_TOKEN":     creds.Envs["AWS_SESSION_TOKEN"],
		"AWS_SECURITY_TOKEN":    creds.Envs["AWS_SESSION_TOKEN"],
	}

	return envs
}

// Envs returns the credentials as AWS_ACCESS_KEY_ID, AWS_SECRET_ACCESS_KEY, AWS_SESSION_TOKEN, and
// AWS_SECURITY_TOKEN. It logs a warning and returns nil when the access key ID or secret access key is empty.
func (creds *AWSCredentials) Envs(
	_ context.Context,
	l log.Logger,
	authProviderCmd string,
) map[string]string {
	var emptyFields []string

	if creds.AccessKeyID == "" {
		emptyFields = append(emptyFields, "ACCESS_KEY_ID")
	}

	if creds.SecretAccessKey == "" {
		emptyFields = append(emptyFields, "SECRET_ACCESS_KEY")
	}

	if len(emptyFields) > 0 {
		l.Warnf("The command %s completed successfully, but AWS credentials"+
			" contains empty required values: %s, nothing is being done.",
			authProviderCmd, strings.Join(emptyFields, ", "))

		return nil
	}

	envs := map[string]string{
		"AWS_ACCESS_KEY_ID":     creds.AccessKeyID,
		"AWS_SECRET_ACCESS_KEY": creds.SecretAccessKey,
		"AWS_SESSION_TOKEN":     creds.SessionToken,
		"AWS_SECURITY_TOKEN":    creds.SessionToken,
	}

	return envs
}
