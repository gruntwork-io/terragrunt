# This file is copied to live/deeply/nested/folder/.terragrunt-stack/child_stack/
# during stack generation; the unit source below is relative to that location.
unit "child_app" {
  source = "../../../../../units/stacked"
  path   = "child_app"
}
