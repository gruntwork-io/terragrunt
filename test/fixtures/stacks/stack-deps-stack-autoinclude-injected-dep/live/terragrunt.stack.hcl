# The stack-level autoinclude overrides the catalog stack's "function" unit with one that declares its
# own autoinclude, wiring it to the "queue" unit of this stack. The override moves the unit to "handler", so the
# catalog's "logs" wiring must follow the override's path.
unit "queue" {
  source = "${get_repo_root()}/units/echo"
  path   = "queue"

  values = {
    value = "queue-arn"
  }
}

stack "fn" {
  source = "${get_repo_root()}/stacks/fn"
  path   = "fn"

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
