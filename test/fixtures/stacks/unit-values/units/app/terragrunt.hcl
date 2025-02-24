
locals {
  data = "payload: ${values.deployment}-${values.project}"
}

inputs = {
  deployment = values.deployment
  project = values.project
  data = local.data
}