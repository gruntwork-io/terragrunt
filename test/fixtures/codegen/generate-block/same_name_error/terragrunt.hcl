generate "backend" {
  path = "data.txt"
  if_exists = "overwrite"
  contents = "test data1"
}

generate "backend" {
  path = "data.txt"
  if_exists = "overwrite"
  contents = "test data2"
}


terraform {
  source = "../../module"
}
