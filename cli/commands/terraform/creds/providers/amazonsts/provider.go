package amazonsts

import (
	"context"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/gruntwork-io/terragrunt/aws_helper"
	"github.com/gruntwork-io/terragrunt/cli/commands/terraform/creds/providers"
	"github.com/gruntwork-io/terragrunt/options"
)

// Provider obtains credentials by making API requests to Amazon STS.
type Provider struct {
	terragruntOptions *options.TerragruntOptions
}

// NewProvider returns a new Provider instance.
func NewProvider(opts *options.TerragruntOptions) providers.Provider {
	return &Provider{
		terragruntOptions: opts,
	}
}

// Name implements providers.Name
func (provider *Provider) Name() string {
	return "API calls to Amazon STS"
}

// GetCredentials implements providers.GetCredentials
func (provider *Provider) GetCredentials(ctx context.Context) (*providers.Credentials, error) {
	iamRoleOpts := provider.terragruntOptions.IAMRoleOptions
	if iamRoleOpts.RoleARN == "" {
		return nil, nil
	}

	provider.terragruntOptions.Logger.Debugf("Assuming IAM role %s with a session duration of %d seconds.", iamRoleOpts.RoleARN, iamRoleOpts.AssumeRoleDuration)
	resp, err := aws_helper.AssumeIamRole(iamRoleOpts)
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
	return creds, nil
}
