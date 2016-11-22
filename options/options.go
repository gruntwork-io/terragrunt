package options

// TerragruntOptions represents command-line options that are read by Terragrunt
type TerragruntOptions struct {
	TerragruntConfigPath string
	NonInteractive       bool
	NonTerragruntArgs    []string
}

