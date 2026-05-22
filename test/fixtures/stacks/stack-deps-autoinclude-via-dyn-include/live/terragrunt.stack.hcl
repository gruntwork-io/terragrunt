// include.path is an HCL expression; the phased parser resolves it and generates autoinclude from the included file.

include "shared" {
  path = format("%s", "./shared.hcl")
}
