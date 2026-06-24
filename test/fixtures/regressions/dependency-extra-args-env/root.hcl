# Local backend so dependency output optimization is eligible.
remote_state {
  backend = "local"

  generate = {
    path      = "backend.tf"
    if_exists = "overwrite_terragrunt"
  }

  config = {
    path = "terraform.tfstate"
  }
}

# Select a non-default workspace via an env var supplied only through extra_arguments.
terraform {
  extra_arguments "workspace" {
    commands = [
      "init",
      "plan",
      "apply",
      "destroy",
      "refresh",
      "state",
      "output",
    ]

    env_vars = {
      TF_WORKSPACE = "custom"
    }
  }
}
