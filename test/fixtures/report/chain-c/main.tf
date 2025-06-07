resource "null_resource" "chain_c" {
  triggers = {
    depends_on_b = dependency.chain_b.outputs
  }
}
