terraform {
  before_hook "before_hook" {
    commands = ["init"]
    execute  = ["./test-creds.sh"]
  }
}

dependency "app3" {
  config_path  = "../app3"
}

inputs = {
  foo-app3 = dependency.app3.outputs.foo-app3
}
