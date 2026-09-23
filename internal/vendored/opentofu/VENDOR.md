# Vendored OpenTofu code

This directory holds code copied from [OpenTofu](https://github.com/opentofu/opentofu), so Terragrunt evaluates HCL with the same built-in function implementations as OpenTofu. OpenTofu keeps these packages under `internal/`, where Go does not allow other modules to import them. If OpenTofu ever ends up publishing them as a library, Terragrunt will probably drop this package to import the shared library.

## Layout

- `upstream/` holds the code copied from OpenTofu. `vend` generates all of it, and nothing in it is edited by hand. CI runs `vend` and fails when the result differs from what is committed. Each file keeps its path in the OpenTofu repository, minus the leading `internal/`.
- `patch/` holds code written by hand for Terragrunt on top of `upstream/`. `vend` never touches it.
- `vend/` is the command that regenerates `upstream/`.

## Source

| | |
| --- | --- |
| Tag | `v1.13.0-beta1` |
| Commit | `cfe442d449412bcc76e9d36f4a0cef19483c3eb2` |

`vend` reads the release to regenerate from this table, and updates it when it vendors a new tag.

## Updating

From the repository root, run:

```sh
mise x go -- go run ./internal/vendored/opentofu/vend --tag <tag>
```

`vend` fetches the tag, deletes `upstream/`, regenerates it, runs `go mod tidy`, `go fix`, and `gofmt`, and builds and tests the packages in it. When that succeeds, it records the tag and the commit the tag resolves to in the [Source](#source) table.

Without `--tag`, `vend` regenerates the release in the Source table, and stops if the tag no longer resolves to the recorded commit.
