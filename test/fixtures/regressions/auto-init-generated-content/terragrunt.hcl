generate "meta" {
  path      = "meta.tf"
  if_exists = "overwrite"
  contents  = <<EOT
locals {
  generated_at = "${uuid()}"
}

output "generated_at" {
  value = local.generated_at
}
EOT
}

generate "module" {
  path      = "module.tf"
  if_exists = "overwrite"
  contents  = <<EOT
module "selected" {
  source = "./${get_env("TG_TEST_AUTO_INIT_MODULE", "a")}"
}
EOT
}
