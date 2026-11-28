package mcp

import (
	"github.com/gruntwork-io/terragrunt/pkg/venv"
)

// CapabilityDocs lets the server relay the tools and resources of the
// Terragrunt docs MCP server.
const CapabilityDocs venv.Capability = "docs"

// allCapabilities is every capability --allow accepts, in the order the error
// for an unknown one lists them.
var allCapabilities = append(venv.RestrictCapabilities(), CapabilityDocs)
