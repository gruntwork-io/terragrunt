locals {
  read_file = path.read
  data      = file("${dirname(path.read)}/shared.txt")
  reads     = [for path in [{ read = "iterator" }] : path.read]
  units     = [for unit in [{ path = "iterator" }] : unit.path]
}
