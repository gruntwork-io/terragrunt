package filter_test

import (
	"fmt"
	"path/filepath"
	"sort"

	"github.com/gruntwork-io/terragrunt/internal/component"
	"github.com/gruntwork-io/terragrunt/internal/filter"
	"github.com/gruntwork-io/terragrunt/pkg/log"
)

// Example_basicPathFilter demonstrates filtering components by path with a glob pattern.
func Example_basicPathFilter() {
	components := []component.Component{
		component.NewUnit("./apps/app1").WithDiscoveryContext(&component.DiscoveryContext{
			WorkingDir: ".",
		}),
		component.NewUnit("./apps/app2").WithDiscoveryContext(&component.DiscoveryContext{
			WorkingDir: ".",
		}),
		component.NewUnit("./libs/db").WithDiscoveryContext(&component.DiscoveryContext{
			WorkingDir: ".",
		}),
	}

	l := log.New()
	result, _ := filter.Apply(l, "./apps/*", components)

	for _, c := range result {
		fmt.Println(filepath.Base(c.Path()))
	}
	// Output:
	// app1
	// app2
}

// Example_attributeFilter demonstrates filtering components by name attribute.
func Example_attributeFilter() {
	components := []component.Component{
		component.NewUnit("./apps/frontend").WithDiscoveryContext(&component.DiscoveryContext{
			WorkingDir: ".",
		}),
		component.NewUnit("./apps/backend").WithDiscoveryContext(&component.DiscoveryContext{
			WorkingDir: ".",
		}),
		component.NewUnit("./services/api").WithDiscoveryContext(&component.DiscoveryContext{
			WorkingDir: ".",
		}),
	}

	l := log.New()
	result, _ := filter.Apply(l, "name=api", components)

	for _, c := range result {
		fmt.Println(c.Path())
	}
	// Output:
	// ./services/api
}

// Example_exclusionFilter demonstrates excluding components using the negation operator.
func Example_exclusionFilter() {
	components := []component.Component{
		component.NewUnit("./apps/app1").WithDiscoveryContext(&component.DiscoveryContext{
			WorkingDir: ".",
		}),
		component.NewUnit("./apps/app2").WithDiscoveryContext(&component.DiscoveryContext{
			WorkingDir: ".",
		}),
		component.NewUnit("./apps/legacy").WithDiscoveryContext(&component.DiscoveryContext{
			WorkingDir: ".",
		}),
	}

	l := log.New()
	result, _ := filter.Apply(l, "!legacy", components)

	for _, c := range result {
		fmt.Println(filepath.Base(c.Path()))
	}
	// Output:
	// app1
	// app2
}

// Example_intersectionFilter demonstrates refining results with the intersection operator.
func Example_intersectionFilter() {
	components := []component.Component{
		component.NewUnit("./apps/frontend").WithDiscoveryContext(&component.DiscoveryContext{
			WorkingDir: ".",
		}),
		component.NewUnit("./apps/backend").WithDiscoveryContext(&component.DiscoveryContext{
			WorkingDir: ".",
		}),
		component.NewUnit("./libs/db").WithDiscoveryContext(&component.DiscoveryContext{
			WorkingDir: ".",
		}),
		component.NewUnit("./libs/api").WithDiscoveryContext(&component.DiscoveryContext{
			WorkingDir: ".",
		}),
	}

	// Select components in ./apps/ that are named "frontend"
	l := log.New()
	result, _ := filter.Apply(l, "./apps/* | frontend", components)

	for _, c := range result {
		fmt.Println(filepath.Base(c.Path()))
	}
	// Output:
	// frontend
}

// Example_complexQuery demonstrates a complex filter combining paths and negation.
func Example_complexQuery() {
	components := []component.Component{
		component.NewUnit("./services/web").WithDiscoveryContext(&component.DiscoveryContext{
			WorkingDir: ".",
		}),
		component.NewUnit("./services/worker").WithDiscoveryContext(&component.DiscoveryContext{
			WorkingDir: ".",
		}),
		component.NewUnit("./libs/db").WithDiscoveryContext(&component.DiscoveryContext{
			WorkingDir: ".",
		}),
		component.NewUnit("./libs/api").WithDiscoveryContext(&component.DiscoveryContext{
			WorkingDir: ".",
		}),
		component.NewUnit("./libs/cache").WithDiscoveryContext(&component.DiscoveryContext{
			WorkingDir: ".",
		}),
	}

	// Select all services except worker
	l := log.New()
	result, _ := filter.Apply(l, "./services/* | !worker", components)

	for _, c := range result {
		fmt.Println(filepath.Base(c.Path()))
	}
	// Output:
	// web
}

// Example_parseAndEvaluate demonstrates the two-step process of parsing and evaluating.
func Example_parseAndEvaluate() {
	components := []component.Component{
		component.NewUnit("./apps/app1").WithDiscoveryContext(&component.DiscoveryContext{
			WorkingDir: ".",
		}),
		component.NewUnit("./apps/app2").WithDiscoveryContext(&component.DiscoveryContext{
			WorkingDir: ".",
		}),
	}

	// Parse the filter once
	f, err := filter.Parse("app1")
	if err != nil {
		fmt.Println("Parse error:", err)
		return
	}

	// Evaluate multiple times with different config sets
	l := log.New()
	result1, _ := f.Evaluate(l, components)
	fmt.Printf("Found %d components\n", len(result1))

	// You can also inspect the original query
	fmt.Printf("Original query: %s\n", f.String())

	// Output:
	// Found 1 components
	// Original query: app1
}

// Example_recursiveWildcard demonstrates using recursive wildcards to match nested paths.
func Example_recursiveWildcard() {
	components := []component.Component{
		component.NewUnit("./infrastructure/networking/vpc").WithDiscoveryContext(&component.DiscoveryContext{
			WorkingDir: ".",
		}),
		component.NewUnit("./infrastructure/networking/subnets").WithDiscoveryContext(&component.DiscoveryContext{
			WorkingDir: ".",
		}),
		component.NewUnit("./infrastructure/compute/app-server").WithDiscoveryContext(&component.DiscoveryContext{
			WorkingDir: ".",
		}),
	}

	// Match all infrastructure components at any depth
	l := log.New()
	result, _ := filter.Apply(l, "./infrastructure/**", components)

	for _, c := range result {
		fmt.Println(filepath.Base(c.Path()))
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
	components := []component.Component{
		component.NewUnit("./apps/app1").WithDiscoveryContext(&component.DiscoveryContext{
			WorkingDir: ".",
		}),
		component.NewUnit("./apps/app2").WithDiscoveryContext(&component.DiscoveryContext{
			WorkingDir: ".",
		}),
		component.NewUnit("./libs/db").WithDiscoveryContext(&component.DiscoveryContext{
			WorkingDir: ".",
		}),
		component.NewUnit("./libs/api").WithDiscoveryContext(&component.DiscoveryContext{
			WorkingDir: ".",
		}),
	}

	// Parse multiple filters - results are unioned
	filters, _ := filter.ParseFilterQueries([]string{
		"./apps/*",
		"name=db",
	})

	l := log.New()
	result, _ := filters.Evaluate(l, components)

	// Sort for consistent output
	names := make([]string, len(result))
	for i, c := range result {
		names[i] = filepath.Base(c.Path())
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
