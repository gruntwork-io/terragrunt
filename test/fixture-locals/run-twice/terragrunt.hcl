locals {
  bar = run_cmd("echo", "foo")
  foo = run_cmd("echo", "bar")

  potato = run_cmd("echo", "potato")
  potato2 = run_cmd("echo", "potato")
}
