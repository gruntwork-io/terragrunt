// Root has only a literal `include` block; the autoinclude block lives in the included file together with parser-incompatible HCL. Generation must fail loudly.

include "shared" {
  path = "./shared.hcl"
}
