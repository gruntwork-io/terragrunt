terraform {
  before_hook "before_hook" {
    commands    = ["validate"]
    execute     = ["ls", "hello_world"]
    working_dir = "${get_terragrunt_dir()}/mydir"
  }
}
