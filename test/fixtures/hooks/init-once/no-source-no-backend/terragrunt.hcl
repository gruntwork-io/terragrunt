terraform {
  # SHOULD execute once. With source always being "." (current directory),
  # init-from-module executes to copy files to cache.
  # If AFTER_INIT_FROM_MODULE_ONLY_ONCE is not present exactly once, the test failed
  after_hook "after_init_from_module" {
    commands = ["init-from-module"]
    execute = ["echo","AFTER_INIT_FROM_MODULE_ONLY_ONCE"]
  }

  # SHOULD execute
  # If AFTER_INIT_ONLY_ONCE is not present exactly once in output, the test failed
  after_hook "after_init" {
    commands = ["init"]
    execute = ["echo","AFTER_INIT_ONLY_ONCE"]
  }
}
