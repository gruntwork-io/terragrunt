package config

import (
	"bytes"
	"github.com/zclconf/go-cty/cty"
	"html/template"
)

type AwsProvider struct {
	Version                   interface{}
	Profile                   interface{}
	Alias                     interface{}
	Region                    interface{}
	AccessKey                 interface{}
	SecretKey                 interface{}
	SharedCredentialsFile     interface{}
	Token                     interface{}
	MaxRetries                interface{}
	AllowedAccountIds         interface{}
	ForbiddenAccountIds       interface{}
	IgnoreTags                interface{}
	SkipCredentialsValidation interface{}
	SkipGetEc2Platforms       interface{}
	SkipRegionValidation      interface{}
	SkipRequestingAccountId   interface{}
	SkipMetadataApiCheck      interface{}
	S3ForcePathStyle          interface{}
	AssumeRole                interface{}
}

type AssumeRole struct {
	DurationSeconds   interface{}
	ExternalId        interface{}
	PolicyArns        interface{}
	RoleArn           interface{}
	TransitiveTagKeys interface{}
}

// awsProviderImplementation "Renders" AWS provider
func awsProviderImplementation(args []cty.Value, retType cty.Type) (cty.Value, error) {
	tpl, err := template.New("aws").Parse(`provider "aws" {
{{if .Alias }}
    alias = "{{.Alias}}"
{{end}}
{{if .Profile }}
    profile = "{{.Profile}}"
{{end}}
{{if .Version }}
    version = "{{.Version}}"
{{end}}
{{if .Region }}
    region = "{{.Region}}"
{{end}}
{{if .SkipCredentialsValidation }}
    skip_credentials_validation = {{.SkipCredentialsValidation}}
{{end}}
}
	`)

	if err != nil {
		panic(err)
	}

	awsProvider := parseProvider(args)

	var tplOutput bytes.Buffer
	err = tpl.Execute(&tplOutput, awsProvider)
	if err != nil {
		panic(err)
	}

	renderedOutput := tplOutput.String()
	return cty.StringVal(renderedOutput), nil
}

// parseProvider parse AwsProvider struct from cty args
func parseProvider(args []cty.Value) AwsProvider {
	var provider AwsProvider
	providerArg := args[0]
	var assumeRole AssumeRole

	bindString(providerArg, &provider.Alias, "alias")
	bindString(providerArg, &provider.Version, "version")
	bindString(providerArg, &provider.AccessKey, "access_key")
	bindString(providerArg, &provider.SecretKey, "secret_key")
	bindString(providerArg, &provider.Region, "region")
	bindString(providerArg, &provider.Profile, "profile")
	bindString(providerArg, &provider.SharedCredentialsFile, "shared_credentials_file")
	bindString(providerArg, &provider.Token, "token")
	bindString(providerArg, &provider.MaxRetries, "max_retries")
	bindStringList(providerArg, &provider.AllowedAccountIds, "allowed_account_ids")
	bindStringList(providerArg, &provider.ForbiddenAccountIds, "forbidden_account_ids")
	bindStringList(providerArg, &provider.IgnoreTags, "ignore_tags")
	bindBool(providerArg, &provider.SkipCredentialsValidation, "skip_credentials_validation")
	bindBool(providerArg, &provider.SkipGetEc2Platforms, "skip_get_ec2_platforms")
	bindBool(providerArg, &provider.SkipRegionValidation, "skip_region_validation")
	bindBool(providerArg, &provider.SkipRequestingAccountId, "skip_requesting_account_id")
	bindBool(providerArg, &provider.SkipMetadataApiCheck, "skip_metadata_api_check")
	bindBool(providerArg, &provider.S3ForcePathStyle, "s3_force_path_style")

	assumeRoleArg := providerArg.GetAttr("assume_role")

	if !assumeRoleArg.IsNull() {
		bindNumber(assumeRoleArg, &assumeRole.DurationSeconds, "duration_seconds")
		bindString(assumeRoleArg, &assumeRole.ExternalId, "external_id")
		bindStringList(assumeRoleArg, &assumeRole.PolicyArns, "policy_arns")
		bindString(assumeRoleArg, &assumeRole.RoleArn, "role_arn")
		bindStringList(assumeRoleArg, &assumeRole.TransitiveTagKeys, "transitive_tag_keys")

		provider.AssumeRole = assumeRole
	}

	return provider
}

// GetAwsProviderHandler Returns AWS provider handler which describes
// provider attributes and provider "rendering" function
func GetAwsProviderHandler() ProviderHandler {
	providerAttrs := cty.ObjectWithOptionalAttrs(map[string]cty.Type{
		"alias":                   cty.String,
		"version":                 cty.String,
		"access_key":              cty.String,
		"secret_key":              cty.String,
		"region":                  cty.String,
		"profile":                 cty.String,
		"shared_credentials_file": cty.String,
		"token":                   cty.String,
		"max_retries":             cty.String,
		"allowed_account_ids":     cty.List(cty.String),
		"forbidden_account_ids":   cty.List(cty.String),
		//"default_tags":            cty.List(cty.String),
		"ignore_tags": cty.List(cty.String),
		"insecure":    cty.Bool,

		// boolean flags block
		"skip_credentials_validation": cty.Bool,
		"skip_get_ec2_platforms":      cty.Bool,
		"skip_region_validation":      cty.Bool,
		"skip_requesting_account_id":  cty.Bool,
		"skip_metadata_api_check":     cty.Bool,
		"s3_force_path_style":         cty.Bool,

		// assume role config
		"assume_role": cty.ObjectWithOptionalAttrs(map[string]cty.Type{
			"duration_seconds":    cty.Number,
			"external_id":         cty.String,
			"policy_arns":         cty.List(cty.String),
			"role_arn":            cty.String,
			"transitive_tag_keys": cty.List(cty.String),
			//"tags": cty.String,
		}, []string{
			"duration_seconds",
			"external_id",
			"policy_arns",
			"role_arn",
			"transitive_tag_keys",
			//"tags",
		}),
	}, []string{
		"alias",
		"version",
		"access_key",
		"secret_key",
		"region",
		"profile",
		"shared_credentials_file",
		"token",
		"max_retries",
		"allowed_account_ids",
		"forbidden_account_ids",
		//"default_tags",
		"ignore_tags",
		"insecure",

		// boolean flags block
		"skip_credentials_validation",
		"skip_get_ec2_platforms",
		"skip_region_validation",
		"skip_requesting_account_id",
		"skip_metadata_api_check",
		"s3_force_path_style",

		"assume_role",
	})

	return ProviderHandler{
		Param: providerAttrs,
		Impl:  awsProviderImplementation,
	}
}
