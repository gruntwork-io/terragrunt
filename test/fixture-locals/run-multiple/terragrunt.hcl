locals {
  bar = run_cmd("echo", "foo")
  foo = run_cmd("echo", "bar")

  potato = run_cmd("echo", "potato")
  potato2 = run_cmd("echo", "potato")

  carrot = run_cmd("echo", "carrot")

  random_arg = run_cmd("echo", "random_arg",  uuid())
  random_arg2 = run_cmd("echo", "random_arg",  uuid())

  uuid = run_cmd("echo", "uuid",  uuid())

  another_arg = run_cmd("echo", "another_arg",  uuid())

}

inputs = {
  fileName = run_cmd("echo", "carrot")
  uuid2 = run_cmd("echo", "uuid", uuid())
  another_arg2 = run_cmd("echo", "another_arg",  uuid())
  input_variable = run_cmd("echo", "input_variable", uuid())
}
