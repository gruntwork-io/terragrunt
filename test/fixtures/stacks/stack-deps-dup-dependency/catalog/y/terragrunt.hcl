terraform {
  source = "."
}

# The unit declares its own dependency with a deliberately wrong path. If autoinclude
# does not override it by name, resolving this path fails (the path does not exist).
dependency "x" {
  config_path = "./this-path-does-not-exist"
}

remote_state {
  backend = "local"
  generate = {
    path      = "backend.tf"
    if_exists = "overwrite_terragrunt"
  }
  config = {
    path = "${dependency.x.outputs.v}.tfstate"
  }
}
