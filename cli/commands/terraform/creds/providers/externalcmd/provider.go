package externalcmd

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/gruntwork-io/go-commons/errors"
	"github.com/gruntwork-io/terragrunt/cli/commands/terraform/creds/providers"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/shell"
	"golang.org/x/exp/maps"
)

// Provider runs external command that returns a json string with credentials.
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
	return fmt.Sprintf("external %s command", provider.terragruntOptions.AuthProviderCmd)
}

// GetCredentials implements providers.GetCredentials
func (provider *Provider) GetCredentials(ctx context.Context) (*providers.Credentials, error) {
	command := provider.terragruntOptions.AuthProviderCmd
	if command == "" {
		return nil, nil
	}

	var args []string
	if parts := strings.Fields(command); len(parts) > 1 {
		command = parts[0]
		args = parts[1:]
	}

	output, err := shell.RunShellCommandWithOutput(ctx, provider.terragruntOptions, "", true, false, command, args...)
	if err != nil {
		return nil, err
	}

	if output.Stdout == "" {
		return nil, errors.Errorf("command %s completed successfully, but the response does not contain JSON string", command)
	}

	resp := &Response{Envs: make(map[string]string)}

	if err := json.Unmarshal([]byte(output.Stdout), &resp); err != nil {
		return nil, errors.Errorf("command %s returned a response with invalid JSON format", command)
	}

	creds := &providers.Credentials{
		Name: providers.AWSCredentials,
		Envs: resp.Envs,
	}

	if resp.AWSCredentials != nil {
		if envs := resp.AWSCredentials.Envs(provider.terragruntOptions); envs != nil {
			provider.terragruntOptions.Logger.Debugf("Obtaining AWS credentials from the %s.", provider.Name())
			maps.Copy(creds.Envs, envs)
		}
	}

	return creds, nil
}

type Response struct {
	AWSCredentials *AWSCredentials   `json:"awsCredentials"`
	Envs           map[string]string `json:"envs"`
}

type AWSCredentials struct {
	AccessKeyID     string `json:"ACCESS_KEY_ID"`
	SecretAccessKey string `json:"SECRET_ACCESS_KEY"`
	SessionToken    string `json:"SESSION_TOKEN"`
}

func (creds *AWSCredentials) Envs(opts *options.TerragruntOptions) map[string]string {
	var emptyFields []string

	if creds.AccessKeyID == "" {
		emptyFields = append(emptyFields, "ACCESS_KEY_ID")
	}
	if creds.SecretAccessKey == "" {
		emptyFields = append(emptyFields, "SECRET_ACCESS_KEY")
	}

	if len(emptyFields) > 0 {
		opts.Logger.Warnf("The command %s completed successfully, but AWS credentials contains empty required values: %s, nothing is being done.", opts.AuthProviderCmd, strings.Join(emptyFields, ", "))
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
