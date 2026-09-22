dependency "hello_world" {
  config_path = "../common-dep"
}

inputs = {
  name = dependency.hello_world.outputs.rendered_template
}

terraform {
  source = "git::__MIRROR_URL__//test/fixtures/download/hello-world?ref=v0.83.2"
}
