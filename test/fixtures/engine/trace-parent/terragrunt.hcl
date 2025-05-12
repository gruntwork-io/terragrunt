terraform {
  source = "."
  before_hook "hook_print_traceparent" {
    commands = ["apply"]
    execute = ["./get_traceparent.sh", "hook_print_traceparent"]
  }
}

engine {
  source  = "github.com/gruntwork-io/terragrunt-engine-opentofu"
  version = "v0.0.16"
  type    = "rpc"
}
