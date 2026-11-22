# Vendored HCL code

`hcldecode/` holds code derived from [HCL](https://github.com/hashicorp/hcl) and changed for Terragrunt. It is kept by hand, and nothing regenerates it.

## Source

| | |
| --- | --- |
| Module | `github.com/hashicorp/hcl/v2` |
| Version | `v2.25.0` |

## Files

- `decode.go` and `plan.go` derive from `gohcl/decode.go` and `gohcl/schema.go`. They decode as `gohcl.DecodeBody` and `gohcl.ImpliedBodySchema` do, but read the hcl tags of each Go type once rather than on every call, and decode attributes and blocks in name order rather than map order.
- `content.go` derives from `(*hclsyntax.Body).PartialContent` in `hclsyntax/structure.go`. It returns the same content and diagnostics without building the body that call returns for the rest, and reads a parsed native body directly.
- `decode_test.go` and `content_test.go` are Terragrunt's own. They test the files above against the upstream functions they derive from.

The derived files keep HCL's copyright notice and its MPL-2.0 license, which [`hcldecode/LICENSE`](hcldecode/LICENSE) holds.

## Updating

When Terragrunt moves to a new release of `github.com/hashicorp/hcl/v2`, compare `gohcl` and `hclsyntax`'s `PartialContent` in that release with the files above, port any change in behavior, and update the version in the [Source](#source) table. The tests compare against the release `go.mod` requires, so they fail on a drift in the behavior they cover.
