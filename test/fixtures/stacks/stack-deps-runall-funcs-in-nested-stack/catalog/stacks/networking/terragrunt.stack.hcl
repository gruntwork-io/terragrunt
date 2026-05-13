// Fixture: nested stack file with ${get_terragrunt_dir()} in unit source; copied verbatim by `stack generate` so discovery must walk it without choking on the function call.

unit "vpc" {
  source = "${get_terragrunt_dir()}/../../../catalog/units/vpc"
  path   = "vpc"
}
