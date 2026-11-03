// Package config is a rewrite of Terragrunt's config parsing that will replace the orchestration in pkg/config. It
// decodes files into block structs of its own and converts them to pkg/config types, reusing pkg/config's merge
// logic and helpers.
//
// Each block struct decodes what a parse can evaluate. The rest stays in Remain until resolve decodes it into Body.
//
// pkg/config must never import this package, which depguard enforces. Delete the package, or merge it into
// pkg/config, once it is the default parser and its parity test has passed across the fixture corpus for one
// release.
package config
