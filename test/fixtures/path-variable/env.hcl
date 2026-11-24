locals {
  read_file    = path.read
  read_unit    = unit.path
  read_include = try(path.include, "not-included")
  data         = file("${dirname(path.read)}/env.txt")
}
