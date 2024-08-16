// Package amazonsts provides tooling for obtaining AWS credentials by making API requests to Amazon STS.
package amazonsts

import (
	"context"
	"errors"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/gruntwork-io/terragrunt/awshelper"
	"github.com/gruntwork-io/terragrunt/cli/commands/terraform/creds/providers"
	"github.com/gruntwork-io/terragrunt/internal/cache"
	"github.com/gruntwork-io/terragrunt/options"
)

// Provider obtains credentials by making API requests to Amazon STS.
type Provider struct {
	terragruntOptions *options.TerragruntOptions
}

// NewProvider returns a new Provider instance.
func NewProvider(opts *options.TerragruntOptions) *Provider {
	return &Provider{
		terragruntOptions: opts,
	}
}

// Name implements providers.Name.
func (provider *Provider) Name() string {
	return "API calls to Amazon STS"
}

var (
	// ErrRoleNotDefined is returned when the IAM role is not defined.
	ErrRoleNotDefined = errors.New("IAM role not defined")
)

// GetCredentials implements providers.GetCredentials.
func (provider *Provider) GetCredentials(ctx context.Context) (*providers.Credentials, error) {
	iamRoleOpts := provider.terragruntOptions.IAMRoleOptions
	if iamRoleOpts.RoleARN == "" {
		return nil, ErrRoleNotDefined
	}

	if cached, hit := credentialsCache.Get(ctx, iamRoleOpts.RoleARN); hit {
		provider.terragruntOptions.Logger.Debugf("Using cached credentials for IAM role %s.", iamRoleOpts.RoleARN)

		return cached, nil
	}

	provider.terragruntOptions.Logger.Debugf("Assuming IAM role %s with a session duration of %d seconds.", iamRoleOpts.RoleARN, iamRoleOpts.AssumeRoleDuration) //nolint:lll

	resp, err := awshelper.AssumeIamRole(iamRoleOpts)
	if err != nil {
		return nil, err
	}

	creds := &providers.Credentials{
		Name: providers.AWSCredentials,
		Envs: map[string]string{
			"AWS_ACCESS_KEY_ID":     aws.StringValue(resp.AccessKeyId),
			"AWS_SECRET_ACCESS_KEY": aws.StringValue(resp.SecretAccessKey),
			"AWS_SESSION_TOKEN":     aws.StringValue(resp.SessionToken),
			"AWS_SECURITY_TOKEN":    aws.StringValue(resp.SessionToken),
		},
	}

	exp := time.Now().Add(time.Duration(iamRoleOpts.AssumeRoleDuration) * time.Second)
	credentialsCache.Put(ctx, iamRoleOpts.RoleARN, creds, exp)

	return creds, nil
}

// credentialsCache is a cache of credentials.
var credentialsCache = cache.NewExpiringCache[*providers.Credentials]("credentialsCache") //nolint:gochecknoglobals
