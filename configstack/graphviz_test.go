package configstack

import (
	"bytes"
	"strings"
	"testing"

	"github.com/gruntwork-io/terragrunt/options"
	"github.com/stretchr/testify/assert"
)

func TestGraph(t *testing.T) {
	a := &TerraformModule{Path: "a"}
	b := &TerraformModule{Path: "b"}
	c := &TerraformModule{Path: "c"}
	d := &TerraformModule{Path: "d"}
	e := &TerraformModule{Path: "e", Dependencies: []*TerraformModule{a}}
	f := &TerraformModule{Path: "f", Dependencies: []*TerraformModule{a, b}}
	g := &TerraformModule{Path: "g", Dependencies: []*TerraformModule{e}}
	h := &TerraformModule{Path: "h", Dependencies: []*TerraformModule{g, f, c}}

	var stdout bytes.Buffer
	terragruntOptions, _ := options.NewTerragruntOptionsForTest("/terragrunt.hcl")
	WriteDot(&stdout, terragruntOptions, []*TerraformModule{a, b, c, d, e, f, g, h})
	expected := strings.TrimSpace(`
digraph {
	"a" ;
	"b" ;
	"c" ;
	"d" ;
	"e" ;
	"e" -> "a";
	"f" ;
	"f" -> "a";
	"f" -> "b";
	"g" ;
	"g" -> "e";
	"h" ;
	"h" -> "g";
	"h" -> "f";
	"h" -> "c";
}
`)
	assert.True(t, strings.Contains(stdout.String(), expected))
}

func TestGraphTrimPrefix(t *testing.T) {
	a := &TerraformModule{Path: "/config/a"}
	b := &TerraformModule{Path: "/config/b"}
	c := &TerraformModule{Path: "/config/c"}
	d := &TerraformModule{Path: "/config/d"}
	e := &TerraformModule{Path: "/config/alpha/beta/gamma/e", Dependencies: []*TerraformModule{a}}
	f := &TerraformModule{Path: "/config/alpha/beta/gamma/f", Dependencies: []*TerraformModule{a, b}}
	g := &TerraformModule{Path: "/config/alpha/g", Dependencies: []*TerraformModule{e}}
	h := &TerraformModule{Path: "/config/alpha/beta/h", Dependencies: []*TerraformModule{g, f, c}}

	var stdout bytes.Buffer
	terragruntOptions, _ := options.NewTerragruntOptionsWithConfigPath("/config/terragrunt.hcl")
	WriteDot(&stdout, terragruntOptions, []*TerraformModule{a, b, c, d, e, f, g, h})
	expected := strings.TrimSpace(`
digraph {
	"a" ;
	"b" ;
	"c" ;
	"d" ;
	"alpha/beta/gamma/e" ;
	"alpha/beta/gamma/e" -> "a";
	"alpha/beta/gamma/f" ;
	"alpha/beta/gamma/f" -> "a";
	"alpha/beta/gamma/f" -> "b";
	"alpha/g" ;
	"alpha/g" -> "alpha/beta/gamma/e";
	"alpha/beta/h" ;
	"alpha/beta/h" -> "alpha/g";
	"alpha/beta/h" -> "alpha/beta/gamma/f";
	"alpha/beta/h" -> "c";
}
`)
	assert.True(t, strings.Contains(stdout.String(), expected))
}

func TestGraphFlagExcluded(t *testing.T) {
	a := &TerraformModule{Path: "a", FlagExcluded: true}
	b := &TerraformModule{Path: "b"}
	c := &TerraformModule{Path: "c"}
	d := &TerraformModule{Path: "d"}
	e := &TerraformModule{Path: "e", Dependencies: []*TerraformModule{a}}
	f := &TerraformModule{Path: "f", FlagExcluded: true, Dependencies: []*TerraformModule{a, b}}
	g := &TerraformModule{Path: "g", Dependencies: []*TerraformModule{e}}
	h := &TerraformModule{Path: "h", Dependencies: []*TerraformModule{g, f, c}}

	var stdout bytes.Buffer
	terragruntOptions, _ := options.NewTerragruntOptionsForTest("/terragrunt.hcl")
	WriteDot(&stdout, terragruntOptions, []*TerraformModule{a, b, c, d, e, f, g, h})
	expected := strings.TrimSpace(`
digraph {
	"a" [color=red];
	"b" ;
	"c" ;
	"d" ;
	"e" ;
	"e" -> "a";
	"f" [color=red];
	"f" -> "a";
	"f" -> "b";
	"g" ;
	"g" -> "e";
	"h" ;
	"h" -> "g";
	"h" -> "f";
	"h" -> "c";
}
`)
	assert.True(t, strings.Contains(stdout.String(), expected))
}
