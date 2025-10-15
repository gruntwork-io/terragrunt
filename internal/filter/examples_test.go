package filter_test

import (
	"fmt"
	"sort"

	"github.com/gruntwork-io/terragrunt/internal/filter"
)

// Example_basicPathFilter demonstrates filtering units by path with a glob pattern.
func Example_basicPathFilter() {
	units := []filter.Unit{
		{Name: "app1", Path: "./apps/app1"},
		{Name: "app2", Path: "./apps/app2"},
		{Name: "db", Path: "./libs/db"},
	}

	result, _ := filter.Apply("./apps/*", units)

	for _, unit := range result {
		fmt.Println(unit.Name)
	}
	// Output:
	// app1
	// app2
}

// Example_attributeFilter demonstrates filtering units by name attribute.
func Example_attributeFilter() {
	units := []filter.Unit{
		{Name: "frontend", Path: "./apps/frontend"},
		{Name: "backend", Path: "./apps/backend"},
		{Name: "api", Path: "./services/api"},
	}

	result, _ := filter.Apply("name=api", units)

	for _, unit := range result {
		fmt.Println(unit.Path)
	}
	// Output:
	// ./services/api
}

// Example_exclusionFilter demonstrates excluding units using the negation operator.
func Example_exclusionFilter() {
	units := []filter.Unit{
		{Name: "app1", Path: "./apps/app1"},
		{Name: "app2", Path: "./apps/app2"},
		{Name: "legacy", Path: "./apps/legacy"},
	}

	result, _ := filter.Apply("!legacy", units)

	for _, unit := range result {
		fmt.Println(unit.Name)
	}
	// Output:
	// app1
	// app2
}

// Example_intersectionFilter demonstrates refining results with the intersection operator.
func Example_intersectionFilter() {
	units := []filter.Unit{
		{Name: "frontend", Path: "./apps/frontend"},
		{Name: "backend", Path: "./apps/backend"},
		{Name: "db", Path: "./libs/db"},
		{Name: "api", Path: "./libs/api"},
	}

	// Select units in ./apps/ that are named "frontend"
	result, _ := filter.Apply("./apps/* | frontend", units)

	for _, unit := range result {
		fmt.Println(unit.Name)
	}
	// Output:
	// frontend
}

// Example_complexQuery demonstrates a complex filter combining paths and negation.
func Example_complexQuery() {
	units := []filter.Unit{
		{Name: "web", Path: "./services/web"},
		{Name: "worker", Path: "./services/worker"},
		{Name: "db", Path: "./libs/db"},
		{Name: "api", Path: "./libs/api"},
		{Name: "cache", Path: "./libs/cache"},
	}

	// Select all services except worker
	result, _ := filter.Apply("./services/* | !worker", units)

	for _, unit := range result {
		fmt.Println(unit.Name)
	}
	// Output:
	// web
}

// Example_parseAndEvaluate demonstrates the two-step process of parsing and evaluating.
func Example_parseAndEvaluate() {
	units := []filter.Unit{
		{Name: "app1", Path: "./apps/app1"},
		{Name: "app2", Path: "./apps/app2"},
	}

	// Parse the filter once
	f, err := filter.Parse("app1", ".")
	if err != nil {
		fmt.Println("Parse error:", err)
		return
	}

	// Evaluate multiple times with different unit sets
	result1, _ := f.Evaluate(units)
	fmt.Printf("Found %d units\n", len(result1))

	// You can also inspect the original query
	fmt.Printf("Original query: %s\n", f.String())

	// Output:
	// Found 1 units
	// Original query: app1
}

// Example_recursiveWildcard demonstrates using recursive wildcards to match nested paths.
func Example_recursiveWildcard() {
	units := []filter.Unit{
		{Name: "vpc", Path: "./infrastructure/networking/vpc"},
		{Name: "subnets", Path: "./infrastructure/networking/subnets"},
		{Name: "app-server", Path: "./infrastructure/compute/app-server"},
	}

	// Match all infrastructure units at any depth
	result, _ := filter.Apply("./infrastructure/**", units)

	for _, unit := range result {
		fmt.Println(unit.Name)
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
	units := []filter.Unit{
		{Name: "app1", Path: "./apps/app1"},
		{Name: "app2", Path: "./apps/app2"},
		{Name: "db", Path: "./libs/db"},
		{Name: "api", Path: "./libs/api"},
	}

	// Parse multiple filters - results are unioned
	filters, _ := filter.ParseFilterQueries([]string{
		"./apps/*",
		"name=db",
	})

	result, _ := filters.Evaluate(units)

	// Sort for consistent output
	names := make([]string, len(result))
	for i, unit := range result {
		names[i] = unit.Name
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
