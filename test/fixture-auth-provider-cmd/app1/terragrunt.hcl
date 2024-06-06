terraform {
  before_hook "before_hook" {
    commands     = ["init"]
    execute      = ["./test-creds.sh"]
  }
}

dependency "app2" {
  config_path  = "../app2"
  skip_outputs = true
}
