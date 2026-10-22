# A catalog stack that nests "fn" and overrides its "function" unit.
unit "queue" {
  source                 = "../..//units/echo"
  update_source_with_cas = true
  path                   = "queue"

  values = {
    value = "queue-arn"
  }
}

stack "fn" {
  source                 = "../..//stacks/fn"
  update_source_with_cas = true
  path                   = "fn"

  autoinclude {
    unit "function" {
      source                 = "../..//units/echo"
      update_source_with_cas = true
      path                   = "handler"

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
