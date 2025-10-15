package filter_test

import (
	"fmt"
	"path/filepath"
	"sort"

	"github.com/gruntwork-io/terragrunt/internal/discoveredconfig"
	"github.com/gruntwork-io/terragrunt/internal/filter"
)

// Example_basicPathFilter demonstrates filtering configs by path with a glob pattern.
func Example_basicPathFilter() {
	configs := []*discoveredconfig.DiscoveredConfig{
		{Path: "./apps/app1", Type: discoveredconfig.ConfigTypeUnit},
		{Path: "./apps/app2", Type: discoveredconfig.ConfigTypeUnit},
		{Path: "./libs/db", Type: discoveredconfig.ConfigTypeUnit},
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
	configs := []*discoveredconfig.DiscoveredConfig{
		{Path: "./apps/frontend", Type: discoveredconfig.ConfigTypeUnit},
		{Path: "./apps/backend", Type: discoveredconfig.ConfigTypeUnit},
		{Path: "./services/api", Type: discoveredconfig.ConfigTypeUnit},
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
	configs := []*discoveredconfig.DiscoveredConfig{
		{Path: "./apps/app1", Type: discoveredconfig.ConfigTypeUnit},
		{Path: "./apps/app2", Type: discoveredconfig.ConfigTypeUnit},
		{Path: "./apps/legacy", Type: discoveredconfig.ConfigTypeUnit},
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
	configs := []*discoveredconfig.DiscoveredConfig{
		{Path: "./apps/frontend", Type: discoveredconfig.ConfigTypeUnit},
		{Path: "./apps/backend", Type: discoveredconfig.ConfigTypeUnit},
		{Path: "./libs/db", Type: discoveredconfig.ConfigTypeUnit},
		{Path: "./libs/api", Type: discoveredconfig.ConfigTypeUnit},
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
	configs := []*discoveredconfig.DiscoveredConfig{
		{Path: "./services/web", Type: discoveredconfig.ConfigTypeUnit},
		{Path: "./services/worker", Type: discoveredconfig.ConfigTypeUnit},
		{Path: "./libs/db", Type: discoveredconfig.ConfigTypeUnit},
		{Path: "./libs/api", Type: discoveredconfig.ConfigTypeUnit},
		{Path: "./libs/cache", Type: discoveredconfig.ConfigTypeUnit},
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
	configs := []*discoveredconfig.DiscoveredConfig{
		{Path: "./apps/app1", Type: discoveredconfig.ConfigTypeUnit},
		{Path: "./apps/app2", Type: discoveredconfig.ConfigTypeUnit},
	}

	// Parse the filter once
	f, err := filter.Parse("app1")
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
	configs := []*discoveredconfig.DiscoveredConfig{
		{Path: "./infrastructure/networking/vpc", Type: discoveredconfig.ConfigTypeUnit},
		{Path: "./infrastructure/networking/subnets", Type: discoveredconfig.ConfigTypeUnit},
		{Path: "./infrastructure/compute/app-server", Type: discoveredconfig.ConfigTypeUnit},
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
	_, err := filter.Parse("name=")
	if err != nil {
		fmt.Println("Error occurred")
	}

	// Valid syntax
	_, err = filter.Parse("name=foo")
	if err == nil {
		fmt.Println("Successfully parsed")
	}

	// Output:
	// Error occurred
	// Successfully parsed
}

// Example_multipleFilters demonstrates using multiple filters with union semantics.
func Example_multipleFilters() {
	configs := []*discoveredconfig.DiscoveredConfig{
		{Path: "./apps/app1", Type: discoveredconfig.ConfigTypeUnit},
		{Path: "./apps/app2", Type: discoveredconfig.ConfigTypeUnit},
		{Path: "./libs/db", Type: discoveredconfig.ConfigTypeUnit},
		{Path: "./libs/api", Type: discoveredconfig.ConfigTypeUnit},
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
