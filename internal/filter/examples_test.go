package filter_test

import (
	"fmt"
	"path/filepath"
	"sort"

	"github.com/gruntwork-io/terragrunt/internal/component"
	"github.com/gruntwork-io/terragrunt/internal/filter"
)

// Example_basicPathFilter demonstrates filtering configs by path with a glob pattern.
func Example_basicPathFilter() {
	configs := []*component.Component{
		{Path: "./apps/app1", Kind: component.Unit},
		{Path: "./apps/app2", Kind: component.Unit},
		{Path: "./libs/db", Kind: component.Unit},
	}

	result, _ := filter.Apply("./apps/*", configs)

	for _, cfg := range result {
		fmt.Println(filepath.Base(cfg.Path))
	}
	// Output:
	// app1
	// app2
}

// Example_attributeFilter demonstrates filtering configs by name attribute.
func Example_attributeFilter() {
	configs := []*component.Component{
		{Path: "./apps/frontend", Kind: component.Unit},
		{Path: "./apps/backend", Kind: component.Unit},
		{Path: "./services/api", Kind: component.Unit},
	}

	result, _ := filter.Apply("name=api", configs)

	for _, cfg := range result {
		fmt.Println(cfg.Path)
	}
	// Output:
	// ./services/api
}

// Example_exclusionFilter demonstrates excluding configs using the negation operator.
func Example_exclusionFilter() {
	configs := []*component.Component{
		{Path: "./apps/app1", Kind: component.Unit},
		{Path: "./apps/app2", Kind: component.Unit},
		{Path: "./apps/legacy", Kind: component.Unit},
	}

	result, _ := filter.Apply("!legacy", configs)

	for _, cfg := range result {
		fmt.Println(filepath.Base(cfg.Path))
	}
	// Output:
	// app1
	// app2
}

// Example_intersectionFilter demonstrates refining results with the intersection operator.
func Example_intersectionFilter() {
	configs := []*component.Component{
		{Path: "./apps/frontend", Kind: component.Unit},
		{Path: "./apps/backend", Kind: component.Unit},
		{Path: "./libs/db", Kind: component.Unit},
		{Path: "./libs/api", Kind: component.Unit},
	}

	// Select configs in ./apps/ that are named "frontend"
	result, _ := filter.Apply("./apps/* | frontend", configs)

	for _, cfg := range result {
		fmt.Println(filepath.Base(cfg.Path))
	}
	// Output:
	// frontend
}

// Example_complexQuery demonstrates a complex filter combining paths and negation.
func Example_complexQuery() {
	configs := []*component.Component{
		{Path: "./services/web", Kind: component.Unit},
		{Path: "./services/worker", Kind: component.Unit},
		{Path: "./libs/db", Kind: component.Unit},
		{Path: "./libs/api", Kind: component.Unit},
		{Path: "./libs/cache", Kind: component.Unit},
	}

	// Select all services except worker
	result, _ := filter.Apply("./services/* | !worker", configs)

	for _, cfg := range result {
		fmt.Println(filepath.Base(cfg.Path))
	}
	// Output:
	// web
}

// Example_parseAndEvaluate demonstrates the two-step process of parsing and evaluating.
func Example_parseAndEvaluate() {
	configs := []*component.Component{
		{Path: "./apps/app1", Kind: component.Unit},
		{Path: "./apps/app2", Kind: component.Unit},
	}

	// Parse the filter once
	f, err := filter.Parse("app1", ".")
	if err != nil {
		fmt.Println("Parse error:", err)
		return
	}

	// Evaluate multiple times with different config sets
	result1, _ := f.Evaluate(configs)
	fmt.Printf("Found %d configs\n", len(result1))

	// You can also inspect the original query
	fmt.Printf("Original query: %s\n", f.String())

	// Output:
	// Found 1 configs
	// Original query: app1
}

// Example_recursiveWildcard demonstrates using recursive wildcards to match nested paths.
func Example_recursiveWildcard() {
	configs := []*component.Component{
		{Path: "./infrastructure/networking/vpc", Kind: component.Unit},
		{Path: "./infrastructure/networking/subnets", Kind: component.Unit},
		{Path: "./infrastructure/compute/app-server", Kind: component.Unit},
	}

	// Match all infrastructure configs at any depth
	result, _ := filter.Apply("./infrastructure/**", configs)

	for _, cfg := range result {
		fmt.Println(filepath.Base(cfg.Path))
	}
	// Output:
	// vpc
	// subnets
	// app-server
}

// Example_errorHandling demonstrates handling parsing errors.
func Example_errorHandling() {
	// Invalid syntax - missing value after =
	_, err := filter.Parse("name=", ".")
	if err != nil {
		fmt.Println("Error occurred")
	}

	// Valid syntax
	_, err = filter.Parse("name=foo", ".")
	if err == nil {
		fmt.Println("Successfully parsed")
	}

	// Output:
	// Error occurred
	// Successfully parsed
}

// Example_multipleFilters demonstrates using multiple filters with union semantics.
func Example_multipleFilters() {
	configs := []*component.Component{
		{Path: "./apps/app1", Kind: component.Unit},
		{Path: "./apps/app2", Kind: component.Unit},
		{Path: "./libs/db", Kind: component.Unit},
		{Path: "./libs/api", Kind: component.Unit},
	}

	// Parse multiple filters - results are unioned
	filters, _ := filter.ParseFilterQueries([]string{
		"./apps/*",
		"name=db",
	})

	result, _ := filters.Evaluate(configs)

	// Sort for consistent output
	names := make([]string, len(result))
	for i, cfg := range result {
		names[i] = filepath.Base(cfg.Path)
	}

	sort.Strings(names)

	for _, name := range names {
		fmt.Println(name)
	}
	// Output:
	// app1
	// app2
	// db
}
