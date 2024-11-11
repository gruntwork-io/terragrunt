package config

type Exclude struct {
	If               bool     `cty:"if" hcl:"if,attr"`
	Actions          []string `cty:"actions" hcl:"actions,attr"`
	SkipDependencies bool     `cty:"skip_dependencies" hcl:"skip_dependencies,attr"`
}
