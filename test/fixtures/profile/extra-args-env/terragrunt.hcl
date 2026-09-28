terraform {
  extra_arguments "profiling" {
    commands = ["init", "plan"]

    env_vars = {
      TOFU_CPU_PROFILE = "extra_args_tofu.prof"
    }
  }
}
