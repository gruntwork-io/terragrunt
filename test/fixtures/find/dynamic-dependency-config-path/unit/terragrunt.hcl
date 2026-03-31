dependency "target" {
  config_path = read_terragrunt_config("does-not-exist.hcl").locals.aws_region == "x" ? "../a" : "../b"
}

terraform {
  source = "."
}
