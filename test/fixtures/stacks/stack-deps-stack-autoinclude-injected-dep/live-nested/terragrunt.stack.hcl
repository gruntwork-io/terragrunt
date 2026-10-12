# The stack-level autoinclude overrides the catalog "wrap" stack's "inner" stack with one that declares its
# own stack-level autoinclude, which in turn overrides "function" two levels down.
unit "queue" {
  source = "${get_repo_root()}/units/echo"
  path   = "queue"

  values = {
    value = "queue-arn"
  }
}

stack "wrap" {
  source = "${get_repo_root()}/stacks/wrap"
  path   = "wrap"

  autoinclude {
    stack "inner" {
      source = "${get_repo_root()}/stacks/fn"
      path   = "inner"

      autoinclude {
        unit "function" {
          source = "${get_repo_root()}/units/echo"
          path   = "handler"

          autoinclude {
            dependency "queue" {
              config_path = unit["queue"].path

              mock_outputs = {
                value = "mock-queue"
              }
            }

            inputs = {
              value = "function:${dependency.queue.outputs.value}"
            }
          }
        }
      }
    }
  }
}
