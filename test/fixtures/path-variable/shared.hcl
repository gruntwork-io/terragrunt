locals {
  read_file = path.read
  data      = file("${dirname(path.read)}/shared.txt")
  units     = [for path in [{ unit = "iterator" }] : path.unit]
}
