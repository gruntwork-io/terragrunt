terragrunt = {
  remote_state {
    backend = "s3"
    config {
      encrypt = true
      bucket = "__FILL_IN_BUCKET_NAME__"
      key = "terraform.tfstate"
      region = "__FILL_IN_REGION__"
    }
  }

  terraform {
    # Should NOT execute. With no source, init-from-module should never execute.
    # If AFTER_INIT_FROM_MODULE_ONLY_ONCE is present in output, the test failed
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
}
