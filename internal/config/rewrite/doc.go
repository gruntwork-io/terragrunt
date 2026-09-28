// Package config is a rewrite of Terragrunt's config parsing that will replace the orchestration in pkg/config. It
// decodes files into block structs of its own and converts them to pkg/config types, reusing pkg/config's merge
// logic and helpers.
//
// A parse fetches no dependency outputs. It decodes the tagged fields of each block struct and the queue parts,
// which [QueueBodies] names, with no `dependency` variable in scope, and keeps both. [UnitConfig.ToV1] fetches the
// outputs, decodes the run parts, which [RunBodies] names, and converts the parse's result and its own together.
// Every part evaluates once, and nothing a parse produced changes afterwards.
//
// pkg/config must never import this package, which depguard enforces. Delete the package, or merge it into
// pkg/config, once it is the default parser and its parity test has passed across the fixture corpus for one
// release.
package config
