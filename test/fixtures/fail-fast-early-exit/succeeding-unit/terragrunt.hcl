terraform {
  after_hook "sleep" {
    commands = ["apply"]
    execute  = ["sleep", "1"]
  }
}
