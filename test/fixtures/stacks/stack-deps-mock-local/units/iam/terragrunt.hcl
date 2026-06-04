terraform {
  source = "."
}

# Inputs live in the unit itself, not in the autoinclude. The autoinclude only contributes the mock
# dependency; the unit consumes that dependency's mock outputs here.
inputs = {
  account_name = dependency.account.outputs.name
  region       = dependency.account.outputs.region
}
