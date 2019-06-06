terraform {
  source = "../hello-world"

  extra_arguments "custom_vars" {
    commands = [
      "apply",
      "plan",
      "import",
      "push",
      "refresh"
    ]

    arguments = [
      "-var-file=${get_tfvars_dir()}/../extra-args/common.tfvars",
      "-var-file=terraform.tfvars"
    ]
  }
}