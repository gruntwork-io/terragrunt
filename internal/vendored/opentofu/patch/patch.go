// Package patch holds Terragrunt's replacements for the vendored OpenTofu
// functions that bypass the Terragrunt venv or logger. Each one is adapted
// from the upstream implementation in ../upstream/lang/funcs and goes through
// the venv instead, so a run can be driven entirely in memory.
package patch
