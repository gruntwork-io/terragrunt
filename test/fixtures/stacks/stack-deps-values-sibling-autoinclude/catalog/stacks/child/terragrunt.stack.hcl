unit "consumer" {
  source = "${get_repo_root()}/catalog/units/consumer"
  path   = "consumer"

  autoinclude {
    dependency "producer" {
      config_path = values.producer_path

      mock_outputs_allowed_terraform_commands = ["validate", "plan"]
      mock_outputs = {
        val = "mock-val"
      }
    }

    inputs = {
      val = dependency.producer.outputs.val
    }
  }
}
