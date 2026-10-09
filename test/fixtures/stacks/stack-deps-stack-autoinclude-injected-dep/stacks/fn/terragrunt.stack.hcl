# A catalog stack whose "logs" unit is wired from its own "function" unit. A consumer overriding
# "function" must not break this wiring.
unit "function" {
  source = "${get_repo_root()}/units/echo"
  path   = "function"

  values = {
    value = "catalog-default"
  }
}

unit "logs" {
  source = "${get_repo_root()}/units/echo"
  path   = "logs"

  autoinclude {
    dependency "function" {
      config_path = unit["function"].path

      mock_outputs = {
        value = "mock-function"
      }
    }

    inputs = {
      value = "logs:${dependency.function.outputs.value}"
    }
  }
}
