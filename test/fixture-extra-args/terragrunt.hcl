terraform {
  extra_arguments "var-files" {
    required_var_files = [
      "extra.tfvars",
    ]

    optional_var_files = [
      "${get_terragrunt_dir()}/${get_env("TF_VAR_env", "dev")}.tfvars",
      "${get_terragrunt_dir()}/${get_env("TF_VAR_region", "us-east-1")}.tfvars",
    ]

    commands = [
      "apply",
      "plan",
      "import",
      "push",
      "refresh",
    ]
  }
}
