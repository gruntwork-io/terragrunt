// include.path is an HCL expression: production parser resolves it, simplified parser cannot. With autoinclude declared in the included file, generation must fail loudly.

include "shared" {
  path = format("%s", "./shared.hcl")
}
