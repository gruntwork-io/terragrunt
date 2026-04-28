// Regression fixture for issue #5980 (include variant): the root stack file has only an include block; the autoinclude block lives in the included file along with an HCL function the simplified two-pass parser cannot decode. Stack generate must fail loudly so the user knows their autoinclude is being silently dropped.

include "shared" {
  path = "./shared.hcl"
}
