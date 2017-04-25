terragrunt = {
  terraform = {
    extra_arguments "test" {
      arguments = [
        "-var-file=terraform.tfvars",
      ]

      commands = [
        "apply",
        "plan",
        "import",
        "push",
        "refresh",
      ]
    }

    extra_arguments "var-files" {
      required_var_files = [
        "extra.tfvars",
      ]

      optional_var_files = [
        "${get_tfvars_dir()}/${get_env("TF_VAR_env", "dev")}.tfvars",
        "${get_tfvars_dir()}/${get_env("TF_VAR_region", "us-east-1")}.tfvars",
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
}
