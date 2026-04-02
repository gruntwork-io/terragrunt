// Package hclparse provides modular HCL parsing for Terragrunt stack files
// with support for two-phase parsing and deferred evaluation.
//
// The package follows a two-phase parsing strategy inspired by the Gruntwork
// Pipelines config package:
//
// Phase 1 (HCL): Parse struct fields directly from HCL using gohcl tags.
// Content that requires additional context (like the autoinclude block body)
// is captured via `hcl:",remain"` for deferred evaluation.
//
// Phase 2 (Resolve): After building an evaluation context with unit/stack
// path references, resolve the deferred content into final structs.
//
// This enables parsing autoinclude blocks in terragrunt.stack.hcl files
// where dependency.config_path references unit.*.path variables that are
// only available after the first parsing pass.
package hclparse
