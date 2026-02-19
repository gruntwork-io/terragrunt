// Package config contains configuration types for the IaC engine.
package config

// Options defines options for the Terragrunt engine.
type Options struct {
	Meta    map[string]any
	Source  string
	Version string
	Type    string
}
