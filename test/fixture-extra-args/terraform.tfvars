terragrunt = {
  terraform = {
    extra_arguments "test" {
      arguments = [
        "-var-file=terraform.tfvars",
        "-var-file=extra.tfvars",
      ]

      commands = [
        "apply",
        "plan",
        "import",
        "push",
        "refresh",
      ]
    }

    extra_arguments "conditional-tfvars" {
      var_files = [
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
