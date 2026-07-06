dependency "module_a" {
  config_path = "../module-a"
}

terraform {
  # During the module-c dependency-output fallback the command is "output", so the source consumes module-a's output;
  # for module-b's own apply it stays static. This forces the fallback to recompute the --source subdir per module.
  source = get_terraform_command() == "output" ? "../../modules//${dependency.module_a.outputs.module_name}" : "../../modules//module-b"
}
