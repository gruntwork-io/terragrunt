locals {
  env_value = get_env("VARIANT", "")
  variant = local.env_value == "" ? "default" : local.env_value
}

terraform {
  extra_arguments "data_directory" {
    commands  = [get_terraform_command()]
    arguments = []
    env_vars = {
      "TF_DATA_DIR" = ".terraform/${local.variant}"
    }
  }

  extra_arguments "state_backend" {
    commands  = ["init"]
    arguments = ["-backend-config=path=terraform-${local.variant}.tfstate"]
  }
}

inputs = {
  content = local.variant
}