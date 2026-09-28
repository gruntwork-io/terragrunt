terraform {
  source = "${values.base_url}//modules/app?ref=${values.ref}"
}

inputs = {
  name   = values.name
  region = try(values.region, "us-east-1")
}
