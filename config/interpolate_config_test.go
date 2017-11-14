package config

import (
	"testing"
	"github.com/davecgh/go-spew/spew"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/util"
)

func TestResolveTerragruntConfig(t *testing.T) {
	str := `
	terragrunt_var_files = [ "./terraform.tfvars" ]
	terragrunt = {
	terraform {
		extra_arguments "custom_vars" {
		  commands = ["${get_terraform_commands_that_need_vars()}"]
		  arguments = [
			"${import_parent_tree("*.tfvars")}",
			"-var-file=secrets.tfvars",
			"-var-file=terraform.tfvars",
		  ]
		}
	  }
	}`
	configPath := "../test/fixture-parent-folders/tfvar-tree/child/" + DefaultTerragruntConfigPath

	_, err := util.ReadFileAsString(configPath)
	if err != nil {
		t.Fatal(err)
	}

	opts := options.TerragruntOptions{TerragruntConfigPath: configPath, NonInteractive: true}
	ti := TerragruntInterpolation{
		Options: &opts,
	}
	v,err := ti.ResolveTerragruntConfig(str)
	if err != nil{
		t.Error(err)
	}
	spew.Dump(v.Terragrunt.Terraform.ExtraArgs)
}
