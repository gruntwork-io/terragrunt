generate "backend" {
  path = "data.txt"
  if_exists = "overwrite"
  contents = "test data1"
}

generate "backend2" {
  path = "data2.txt"
  if_exists = "overwrite"
  contents = "test data2"
}

generate "backend" {
  path = "data.txt"
  if_exists = "overwrite"
  contents = "test data1"
}

generate "backend2" {
  path = "data2.txt"
  if_exists = "overwrite"
  contents = "test data2"
}


terraform {
  source = "../../module"
}
