terraform {
  before_hook "before_hook" {
    commands = ["init"]
    execute  = ["./test-creds.sh", get_terragrunt_dir()]
    working_dir = dirname(find_in_parent_folders("root.hcl"))
  }
}
