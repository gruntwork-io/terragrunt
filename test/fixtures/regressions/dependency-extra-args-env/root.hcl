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

# State encryption whose passphrase comes from a variable, not a literal.
generate "encryption" {
  path      = "_encryption.tf"
  if_exists = "overwrite_terragrunt"
  contents  = <<-EOF
    terraform {
      encryption {
        key_provider "pbkdf2" "default" {
          passphrase    = var.state_passphrase
          key_length    = 32
          iterations    = 600000
          salt_length   = 32
          hash_function = "sha512"
        }

        method "aes_gcm" "default" {
          keys = key_provider.pbkdf2.default
        }

        state {
          enforced = true
          method   = method.aes_gcm.default
        }

        plan {
          enforced = true
          method   = method.aes_gcm.default
        }
      }
    }
  EOF
}

# Declare the passphrase variable that the encryption block references.
generate "passphrase_variable" {
  path      = "_variables.tf"
  if_exists = "overwrite"
  contents  = <<-EOF
    variable "state_passphrase" {
      type      = string
      sensitive = true
    }
  EOF
}

# Supply the passphrase to tofu only through extra_arguments env_vars.
terraform {
  extra_arguments "secrets" {
    commands = [
      "apply",
      "destroy",
      "import",
      "init",
      "output",
      "plan",
      "refresh",
      "state",
    ]

    env_vars = {
      TF_VAR_state_passphrase = "test-passphrase-1234567890"
    }
  }
}
