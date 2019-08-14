remote_state {
  backend = "s3"
  config = {
    encrypt = true
    bucket = "__FILL_IN_BUCKET_NAME__"
    key = "terraform.tfstate"
    region = "__FILL_IN_REGION__"
  }
}

terraform {
  source = "../base-module"

  after_hook "backend" {
    commands = ["init-from-module"]
    execute  = ["cp", "${get_terragrunt_dir()}/../backend.tf", "."]
  }

  # SHOULD execute.
  # If AFTER_INIT_FROM_MODULE_ONLY_ONCE is not echoed exactly once, the test failed
  after_hook "after_init_from_module" {
    commands = ["init-from-module"]
    execute = ["echo","AFTER_INIT_FROM_MODULE_ONLY_ONCE"]
  }

  # SHOULD execute.
  # If AFTER_INIT_ONLY_ONCE is not echoed exactly once, the test failed
  after_hook "after_init" {
    commands = ["init"]
    execute = ["echo","AFTER_INIT_ONLY_ONCE"]
  }
}

