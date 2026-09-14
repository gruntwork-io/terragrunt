// Package amazonsts provides a credentials provider that obtains credentials by making API requests to Amazon STS.
package amazonsts

import (
	"cmp"
	"context"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sts/types"
	"github.com/gruntwork-io/terragrunt/internal/awshelper"
	"github.com/gruntwork-io/terragrunt/internal/cache"
	"github.com/gruntwork-io/terragrunt/internal/iam"
	"github.com/gruntwork-io/terragrunt/internal/runner/run/creds/providers"
	"github.com/gruntwork-io/terragrunt/internal/telemetry"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/pkg/log"
)

// credentialsCacheExpiryWindow expires the cache slightly before the STS session.
const credentialsCacheExpiryWindow = 5 * time.Minute

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

	if cached, hit := credentialsCache.Get(ctx, iamRoleOpts.RoleARN); hit {
		l.Debugf("Using cached credentials for IAM role %s.", iamRoleOpts.RoleARN)
		return cached, nil
	}

	sessionDurationSecs := cmp.Or(iamRoleOpts.AssumeRoleDuration, int64(iam.DefaultAssumeRoleDuration))

	l.Debugf("Assuming IAM role %s with a session duration of %d seconds.",
		iamRoleOpts.RoleARN, sessionDurationSecs)

	var resp *types.Credentials

	err := telemetry.TelemeterFromContext(ctx).Collect(ctx, l, "creds_assume_role", map[string]any{
		"role_arn":     iamRoleOpts.RoleARN,
		"session_name": iamRoleOpts.AssumeRoleSessionName,
		"duration":     sessionDurationSecs,
	}, func(ctx context.Context, l log.Logger) error {
		var assumeErr error

		resp, assumeErr = awshelper.AssumeIamRole(ctx, v, iamRoleOpts, "")

		return assumeErr
	})
	if err != nil {
		return nil, err
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

	sessionDuration := time.Duration(sessionDurationSecs) * time.Second

	cacheTTL := sessionDuration - credentialsCacheExpiryWindow
	if cacheTTL <= 0 {
		cacheTTL = sessionDuration
	}

	credentialsCache.Put(ctx, iamRoleOpts.RoleARN, creds, time.Now().Add(cacheTTL))

	return creds, nil
}

// credentialsCache is a cache of credentials.
var credentialsCache = cache.NewExpiringCache[*providers.Credentials]("credentialsCache")
