// Root has only a literal `include` block; autoinclude lives in the included file together with production-only HCL on unrelated units.

include "shared" {
  path = "./shared.hcl"
}
