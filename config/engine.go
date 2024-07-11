package config

import (
	"github.com/zclconf/go-cty/cty"
)

// EngineConfig represents the structure of the HCL data
type EngineConfig struct {
	Source  string     `hcl:"source,attr" cty:"source"`
	Version *string    `hcl:"version,attr" cty:"version"`
	Type    *string    `hcl:"type,attr" cty:"type"`
	Meta    *cty.Value `hcl:"meta,attr" cty:"meta"`
}

// Clone returns a copy of the EngineConfig used in deep copy
func (c *EngineConfig) Clone() *EngineConfig {
	return &EngineConfig{
		Source:  c.Source,
		Version: c.Version,
		Type:    c.Type,
		Meta:    c.Meta,
	}
}

// Merge merges the EngineConfig with another EngineConfig
func (c *EngineConfig) Merge(engine *EngineConfig) {
	if engine.Source != "" {
		c.Source = engine.Source
	}
	if engine.Version != nil {
		c.Version = engine.Version
	}
	if engine.Type != nil {
		c.Type = engine.Type
	}
	if engine.Meta != nil {
		c.Meta = engine.Meta
	}
}
