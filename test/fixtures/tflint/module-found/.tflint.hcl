config {
  call_module_type = "all"
}

plugin "terraform" {
  enabled = true
  version = "0.2.1"
  source  = "github.com/terraform-linters/tflint-ruleset-terraform"
}
