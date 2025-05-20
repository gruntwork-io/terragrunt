terraform {
  source = "."
  before_hook "hook_print_traceparent" {
    commands = ["apply"]
    execute = ["./get_traceparent.sh", "hook_print_traceparent"]
  }
}
