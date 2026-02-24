locals {
  marked1 = mark_as_read("foo.txt")
  marked2 = mark_as_read("foo.txt")
  marked3 = mark_as_read("bar.txt")
}

terraform {
  source = "."
}
