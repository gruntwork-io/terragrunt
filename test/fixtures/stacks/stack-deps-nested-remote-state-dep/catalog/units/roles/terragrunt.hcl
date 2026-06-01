terraform {
  source = "."
}

# Mirrors the issue: a generate block referencing a dependency output (this already worked).
generate "provider" {
  path      = "provider.tf"
  if_exists = "overwrite"
  contents  = <<-EOF
    # account id from dependency: ${dependency.account.outputs.id}
  EOF
}

# Mirrors the issue: remote_state referencing a dependency output (this used to fail).
remote_state {
  backend = "local"
  generate = {
    path      = "backend.tf"
    if_exists = "overwrite_terragrunt"
  }
  config = {
    path = "${dependency.account.outputs.name}/roles.tfstate"
  }
}
