locals {
  unit_common_tg = read_terragrunt_config(format(
    "%s/../common/%s/unit-common.hcl",
    get_terragrunt_dir(),
    basename(get_terragrunt_dir()),
  ))
}

terraform {
  source = format("%s/../module", get_terragrunt_dir())
}

generate "backend_local" {
  path      = "backend.tf"
  if_exists = "overwrite_terragrunt"
  contents  = <<-EOF
  terraform {
    backend "local" {
      path = "${get_terragrunt_dir()}/terraform.tfstate"
    }
  }
  EOF
}

inputs = {
  unit_name = basename(get_terragrunt_dir())
}
