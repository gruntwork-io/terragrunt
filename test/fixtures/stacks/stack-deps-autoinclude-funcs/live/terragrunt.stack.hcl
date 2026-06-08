unit "data" {
  source = "${get_repo_root()}/units/data"
  path   = "data"
}

unit "app" {
  source = "${get_repo_root()}/units/app"
  path   = "app"

  autoinclude {
    dependency "data" {
      # read_terragrunt_config is a generate-time function: it is evaluated in the stack file context, so
      # config_path resolves to the sibling unit path at generate time (not deferred).
      config_path = read_terragrunt_config("${get_repo_root()}/live/pointers.hcl").locals.data_config_path

      mock_outputs_allowed_terraform_commands = ["validate", "plan"]
      mock_outputs = {
        value = "mock-data"
      }
    }

    inputs = {
      # A dependency output stays verbatim and resolves inside the unit (from the mock at plan time).
      data_value = dependency.data.outputs.value
      # A function call with no dependency.* reference resolves at generate time in the stack file context.
      greeting = run_cmd("echo", "hi-from-unit")
    }
  }
}
