terraform {
  before_hook "hook_exit_nonzero" {
    commands = ["apply", "plan"]
    execute  = ["sh", "-c", "echo 'lint warning: something is wrong' >&2 && exit 2"]
  }
}
