// Package md is Terragrunt's entrypoint for Markdown: rendering a document as
// styled terminal output, and reading one back as the structure a reader sees.
//
// The Markdown libraries Terragrunt builds on are reached through this package
// and nowhere else, so what those libraries offer is chosen here rather than
// spread across the callers, and replacing one is a change to this package
// rather than to every place a document is rendered or read.
package md
