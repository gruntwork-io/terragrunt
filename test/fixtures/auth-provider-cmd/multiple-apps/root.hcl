terraform {
  before_hook "before_hook" {
    commands = ["init"]
    execute  = ["./test-creds.sh"]
    working_dir = get_terragrunt_dir()
  }
}
