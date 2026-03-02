package engine

// EngineOptions groups CLI-supplied engine options.
type EngineOptions struct {
	// CachePath is the path to the cache directory for engine files.
	CachePath string
	// LogLevel is the custom log level for engine.
	LogLevel string
	// SkipChecksumCheck skips checksum verification for engine packages.
	SkipChecksumCheck bool
	// NoEngine disables IaC engines even when the iac-engine experiment is enabled.
	NoEngine bool
}

// EngineConfig represents the configurations for a Terragrunt engine.
type EngineConfig struct {
	Meta    map[string]any
	Source  string
	Version string
	Type    string
}
