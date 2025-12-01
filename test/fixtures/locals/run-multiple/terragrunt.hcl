locals {
  bar = run_cmd("echo", "echo_foo")
  foo = run_cmd("echo", "echo_bar")

  potato = run_cmd("echo", "echo_potato")
  potato2 = run_cmd("echo", "echo_potato")

  carrot = run_cmd("echo", "echo_carrot")

  random_arg = run_cmd("echo", "echo_random_arg",  uuid())
  random_arg2 = run_cmd("echo", "echo_random_arg",  uuid())

  uuid = run_cmd("echo", "echo_uuid_locals",  uuid())

  another_arg = run_cmd("echo", "echo_another_arg",  uuid())

}

inputs = {
  fileName = run_cmd("echo", "echo_carrot")
  uuid2 = run_cmd("echo", "echo_uuid_input", uuid())
  another_arg2 = run_cmd("echo", "echo_another_arg",  uuid())
  input_variable = run_cmd("echo", "echo_input_variable", uuid())
}
