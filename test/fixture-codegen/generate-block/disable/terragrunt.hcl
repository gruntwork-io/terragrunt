generate "backend" {
  path = "data.txt"
  contents = "test data1"
  if_exists = "error"
  disable = true
}

terraform {
  source = "../../module"
}
