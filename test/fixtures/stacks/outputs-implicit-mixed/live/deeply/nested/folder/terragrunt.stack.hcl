unit "nested_app" {
  source = "../../../units/stacked"
  path   = "nested_app"
}

stack "child_stack" {
  source = "../../../../stacks/child"
  path   = "child_stack"
}
