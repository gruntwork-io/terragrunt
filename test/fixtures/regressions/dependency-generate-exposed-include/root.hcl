locals {
  module_source = "${get_parent_terragrunt_dir()}/module"
}

generate "gen" {
  path      = "generated.tf"
  if_exists = "overwrite_terragrunt"
  contents  = <<EOF
%{ if try(dependency.dep.outputs.value, "") != "" }
locals { from_dep = "${dependency.dep.outputs.value}" }
%{ endif }
EOF
}
