# A dependency between units at different stack levels: the parent passes
# unit.producer.path down to the child stack via values, and a unit inside the
# child stack uses it as its autoinclude dependency config_path.
unit "producer" {
  source = "../catalog/units/producer"
  path   = "producer"
}

stack "child" {
  source = "../catalog/stacks/child"
  path   = "child"

  values = {
    producer_path = unit.producer.path
  }
}
