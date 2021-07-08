package config

import "github.com/zclconf/go-cty/cty"

func awsProviderImplementation(args []cty.Value, retType cty.Type) (cty.Value, error) {
	return cty.StringVal(""), nil
}

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
			"transitive_tag_keys": cty.String,
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
