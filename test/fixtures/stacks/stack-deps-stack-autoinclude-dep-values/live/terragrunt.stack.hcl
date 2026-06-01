# Unsupported pattern (RFC comment #19): a STACK-level autoinclude declares a
# dependency block AND injects a unit whose values derive from that dependency's
# outputs. Dependency outputs are not available at stack generate time, so this
# must fail with a clear typed error pointing at the supported cross-level pattern.
unit "producer" {
  source = "../units/producer"
  path   = "producer"
}

stack "net" {
  source = "../stacks/networking"
  path   = "net"

  autoinclude {
    dependency "producer" {
      config_path = unit.producer.path

      mock_outputs_allowed_terraform_commands = ["validate", "plan"]
      mock_outputs = {
        val = "mock-val"
      }
    }

    unit "extra" {
      source = "${get_repo_root()}/units/extra"
      path   = "extra"

      values = {
        v = dependency.producer.outputs.val
      }
    }
  }
}
