package config

// Unit represents a list of units.
type Unit struct {
	Name   string `cty:"name"    hcl:",label"`
	Source string `hcl:"source,attr" cty:"source"`
	Path   string `hcl:"source,attr" cty:"source"`
}
