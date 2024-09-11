terraform {
  source = "../base-module"

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
