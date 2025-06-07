resource "null_resource" "chain_b" {
  triggers = {
    depends_on_a = dependency.chain_a.outputs
  }
}
