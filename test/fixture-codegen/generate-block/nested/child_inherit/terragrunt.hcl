generate "random_file" {
  path = "random_file.txt"
  if_exists = "overwrite"
  contents = "Hello world"
}
