dependency "hello_world" {
  config_path = "../common-dep"
}

inputs = {
  name = dependency.hello_world.outputs.rendered_template
}

terraform {
  source = "github.com/gruntwork-io/terragrunt.git//test/fixture-download/hello-world?ref=v0.9.9"
}
