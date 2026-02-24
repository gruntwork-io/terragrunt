locals {
  marked = mark_as_read("foo.txt")
}

terraform {
  source = "."
}
