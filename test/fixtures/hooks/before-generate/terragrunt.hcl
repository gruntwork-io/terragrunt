terraform {
  source = "./base-module"

  before_hook "before_generate" {
    commands = ["generate"]
    execute  = ["bash", "-c", "ls tgen_* >/dev/null 2>&1 || echo BEFORE_GENERATE_NO_TGEN_YET"]
  }

  after_hook "after_generate" {
    commands = ["generate"]
    execute  = ["bash", "-c", "ls tgen_* && echo AFTER_GENERATE_TGEN_PRESENT"]
  }
}

generate "providers" {
  path      = "tgen_providers.tf"
  if_exists = "overwrite_terragrunt"
  contents  = <<EOF
# Placeholder generated provider config for the before/after generate hook test.
EOF
}
