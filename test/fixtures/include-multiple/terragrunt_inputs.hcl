inputs = {
  attribute     = "hello"
  old_attribute = "old val"
  list_attr     = ["hello"]
  map_attr = {
    foo  = "bar"
    test = dependency.vpc.outputs.new_attribute
  }
}
