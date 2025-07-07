unit "foo" {
  source = "${find_in_parent_folders("units/basic")}"
  path   = "foo"
}

unit "bar" {
  source = "${find_in_parent_folders("units/basic")}"
  path   = "bar"
}

unit "baz" {
  source = "${find_in_parent_folders("units/basic")}"
  path   = "baz"
}
