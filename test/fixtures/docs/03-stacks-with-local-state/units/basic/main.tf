resource "null_resource" "basic" {
  triggers = {
    hello = "world"
  }
}
