// Nested stack file: uses ${get_terragrunt_dir()} in its unit `source` attribute.
// This file is copied VERBATIM by `stack generate` into .terragrunt-stack/networking/terragrunt.stack.hcl.
// During `run --all`, discovery walks that generated file and parses it; the function call must not
// crash the parser (regression test for issue #5663 comment 4407441298).
//
// Path math: when this file lives at <root>/.terragrunt-stack/networking/, get_terragrunt_dir() is
// <root>/.terragrunt-stack/networking/, so ../../../catalog/units/vpc resolves to <root>/../catalog/units/vpc.

unit "vpc" {
  source = "${get_terragrunt_dir()}/../../../catalog/units/vpc"
  path   = "vpc"
}
