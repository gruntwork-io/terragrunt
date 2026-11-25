locals {
  inner     = read_terragrunt_config("${dirname(path.read)}/inner.hcl")
  read_file = path.read
}
