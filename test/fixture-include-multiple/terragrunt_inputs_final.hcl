inputs = {
  attribute     = "mock"
  new_attribute = "new val"
  list_attr     = ["hello", "mock", "foo"]
  map_attr = {
    bar  = "baz"
    foo  = "bar"
    test = dependency.vpc.outputs.new_attribute
  }
}
