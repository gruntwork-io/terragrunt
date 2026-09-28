
# A provider has to be installed for the test to be meaningful: Terragrunt only
# skips `init` once the provider directory exists in the data dir.
generate "provider" {
  path = "provider.tf"
  if_exists = "overwrite_terragrunt"
  contents = <<EOF
terraform {
  required_providers {
    null = {
      source  = "registry.opentofu.org/hashicorp/null"
      version = "3.2.4"
    }
  }
}

provider "null" {}
EOF
}

terraform {
  source = "."
  extra_arguments "common_vars" {
    commands = get_terraform_commands_that_need_vars()
    arguments = [
      "-var", "test=qwe",
    ]
  }
}
