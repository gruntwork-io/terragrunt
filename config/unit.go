package config

// StackConfigFile represents the structure of terragrunt stack file
type StackConfigFile struct {
	Locals *terragruntLocal `cty:"locals"  hcl:"locals,block"`
	Units  []*Unit          `cty:"unit" hcl:"unit,block"`
}

// Unit represents a list of units.
type Unit struct {
	Name   string `cty:"name"    hcl:",label"`
	Source string `hcl:"source,attr" cty:"source"`
	Path   string `hcl:"source,attr" cty:"source"`
}
