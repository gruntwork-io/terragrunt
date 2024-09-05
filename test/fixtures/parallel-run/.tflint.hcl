plugin "aws-cis" {
  enabled = true
  version = "0.0.2"
  source  = "github.com/gruntwork-io/tflint-ruleset-aws-cis"
}

config {
  module = true
}

plugin "terraform" {
  enabled = false
}
