
locals {
  data = "payload: ${unit.values.deployment}-${unit.values.project}"
}

inputs = {
  deployment = unit.values.deployment
  project = unit.values.project
  data = local.data
}