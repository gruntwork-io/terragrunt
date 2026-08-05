terraform {
  source = "."
}

inputs = {
  vpc_id = values.vpc_id
  cidr   = values.cidr
}
