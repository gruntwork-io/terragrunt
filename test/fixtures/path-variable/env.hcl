locals {
  read_file    = path.read
  read_unit    = path.unit
  read_include = try(path.include, "not-included")
  data         = file("${dirname(path.read)}/env.txt")
}
