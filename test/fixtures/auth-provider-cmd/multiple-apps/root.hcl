terraform {
  before_hook "before_hook" {
    commands = ["init"]
    execute  = ["${get_parent_terragrunt_dir()}/test-creds.sh"]
    working_dir = get_terragrunt_dir()
  }
}
