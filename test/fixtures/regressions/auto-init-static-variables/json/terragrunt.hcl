generate "vars" {
  path      = "vars.tf"
  if_exists = "overwrite"
  contents  = <<EOT
variable "src" {
  default = "./${get_env("TG_TEST_AUTO_INIT_MODULE", "a")}"
}
EOT
}
