package config

import (
	"fmt"

	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/gocty"
)

// Unit represents a list of units.
type Unit struct {
	Name   string `cty:"name"    hcl:",label"`
	Source string `hcl:"source,attr" cty:"source"`
	Path   string `hcl:"path,attr" cty:"path"`
}

// ToCtyValue converts StackConfigFile to cty.Value
func (s *StackConfigFile) ToCtyValue() (cty.Value, error) {
	return gocty.ToCtyValue(s, cty.Object(map[string]cty.Type{
		"locals": cty.Object(map[string]cty.Type{
			// Define locals structure here
		}),
		"unit": cty.List(cty.Object(map[string]cty.Type{
			"name":   cty.String,
			"source": cty.String,
			"path":   cty.String,
		})),
	}))
}

// FromCtyValue converts cty.Value back to StackConfigFile
func FromCtyValue(v cty.Value) (*StackConfigFile, error) {
	var config StackConfigFile
	err := gocty.FromCtyValue(v, &config)

	if err != nil {
		return nil, fmt.Errorf("failed to decode cty value: %w", err)
	}

	return &config, nil
}

// Clone creates a deep copy of Unit.
func (u *Unit) Clone() *Unit {
	if u == nil {
		return nil
	}

	return &Unit{
		Name:   u.Name,
		Source: u.Source,
		Path:   u.Path,
	}
}
