package filter_test

import (
	"fmt"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/component"
	"github.com/gruntwork-io/terragrunt/internal/filter"
)

// BenchmarkIntersectionSerial benchmarks serial intersection evaluation
func BenchmarkIntersectionSerial(b *testing.B) {
	// Force serial evaluation
	originalFlag := filter.DisableParallelization
	filter.DisableParallelization = true
	defer func() { filter.DisableParallelization = originalFlag }()

	components := generateLargeComponentList(1000)
	filterStr := "./apps/* | type=unit | external=false"

	for b.Loop() {
		_, err := filter.Apply(filterStr, components)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkIntersectionStreaming benchmarks streaming intersection evaluation
func BenchmarkIntersectionStreaming(b *testing.B) {
	// Ensure parallel evaluation is enabled
	originalFlag := filter.DisableParallelization
	filter.DisableParallelization = false
	defer func() { filter.DisableParallelization = originalFlag }()

	components := generateLargeComponentList(1000)
	filterStr := "./apps/* | type=unit | external=false"

	for b.Loop() {
		_, err := filter.Apply(filterStr, components)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkFiltersUnionSerial benchmarks serial union evaluation
func BenchmarkFiltersUnionSerial(b *testing.B) {
	// Force serial evaluation
	originalFlag := filter.DisableParallelization
	filter.DisableParallelization = true
	defer func() { filter.DisableParallelization = originalFlag }()

	components := generateLargeComponentList(1000)
	filterStrings := []string{
		"./apps/*",
		"type=unit",
		"external=true",
		"./libs/*",
		"name=db",
	}

	filters, err := filter.ParseFilterQueries(filterStrings)
	if err != nil {
		b.Fatal(err)
	}

	for b.Loop() {
		_, err := filters.Evaluate(components)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkFiltersUnionParallel benchmarks parallel union evaluation
func BenchmarkFiltersUnionParallel(b *testing.B) {
	// Ensure parallel evaluation is enabled
	originalFlag := filter.DisableParallelization
	filter.DisableParallelization = false
	defer func() { filter.DisableParallelization = originalFlag }()

	components := generateLargeComponentList(1000)
	filterStrings := []string{
		"./apps/*",
		"type=unit",
		"external=true",
		"./libs/*",
		"name=db",
	}

	filters, err := filter.ParseFilterQueries(filterStrings)
	if err != nil {
		b.Fatal(err)
	}

	for b.Loop() {
		_, err := filters.Evaluate(components)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkPathFilterSerial benchmarks serial path filter evaluation
func BenchmarkPathFilterSerial(b *testing.B) {
	// Force serial evaluation
	originalFlag := filter.DisableParallelization
	filter.DisableParallelization = true
	defer func() { filter.DisableParallelization = originalFlag }()

	components := generateLargeComponentList(1000)
	filterStr := "./apps/**/*"

	for b.Loop() {
		_, err := filter.Apply(filterStr, components)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkPathFilterParallel benchmarks parallel path filter evaluation
func BenchmarkPathFilterParallel(b *testing.B) {
	// Ensure parallel evaluation is enabled
	originalFlag := filter.DisableParallelization
	filter.DisableParallelization = false
	defer func() { filter.DisableParallelization = originalFlag }()

	components := generateLargeComponentList(1000)
	filterStr := "./apps/**/*"

	for b.Loop() {
		_, err := filter.Apply(filterStr, components)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkParseFilterQueriesSerial benchmarks serial filter parsing
func BenchmarkParseFilterQueriesSerial(b *testing.B) {
	filterStrings := []string{
		"./apps/*",
		"type=unit",
		"external=true",
		"./libs/*",
		"name=db",
		"!legacy",
		"./infra/**/*",
		"type=stack",
	}

	for b.Loop() {
		_, err := filter.ParseFilterQueries(filterStrings)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkParseFilterQueriesParallel benchmarks parallel filter parsing
func BenchmarkParseFilterQueriesParallel(b *testing.B) {
	filterStrings := []string{
		"./apps/*",
		"type=unit",
		"external=true",
		"./libs/*",
		"name=db",
		"!legacy",
		"./infra/**/*",
		"type=stack",
	}

	for b.Loop() {
		_, err := filter.ParseFilterQueries(filterStrings)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkSmallComponentList benchmarks with small component lists (should use serial)
func BenchmarkSmallComponentList(b *testing.B) {
	components := generateLargeComponentList(50) // Below parallel threshold
	filterStr := "./apps/* | type=unit"

	for b.Loop() {
		_, err := filter.Apply(filterStr, components)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkMediumComponentList benchmarks with medium component lists
func BenchmarkMediumComponentList(b *testing.B) {
	components := generateLargeComponentList(200)
	filterStr := "./apps/* | type=unit | external=false"

	for b.Loop() {
		_, err := filter.Apply(filterStr, components)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkLargeComponentList benchmarks with large component lists
func BenchmarkLargeComponentList(b *testing.B) {
	components := generateLargeComponentList(1000)
	filterStr := "./apps/* | type=unit | external=false"

	for b.Loop() {
		_, err := filter.Apply(filterStr, components)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkMassiveSerial benchmarks massive dataset with serial evaluation
func BenchmarkMassiveSerial(b *testing.B) {
	// Force serial evaluation
	originalFlag := filter.DisableParallelization
	filter.DisableParallelization = true
	defer func() { filter.DisableParallelization = originalFlag }()

	components := generateLargeComponentList(20000)
	filterStr := "./apps/* | type=unit | external=false"

	for b.Loop() {
		_, err := filter.Apply(filterStr, components)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkMassiveParallel benchmarks massive dataset with parallel evaluation
func BenchmarkMassiveParallel(b *testing.B) {
	// Ensure parallel evaluation is enabled
	originalFlag := filter.DisableParallelization
	filter.DisableParallelization = false
	defer func() { filter.DisableParallelization = originalFlag }()

	components := generateLargeComponentList(20000)
	filterStr := "./apps/* | type=unit | external=false"

	for b.Loop() {
		_, err := filter.Apply(filterStr, components)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkMassiveUnionSerial benchmarks massive union with serial evaluation
func BenchmarkMassiveUnionSerial(b *testing.B) {
	// Force serial evaluation
	originalFlag := filter.DisableParallelization
	filter.DisableParallelization = true
	defer func() { filter.DisableParallelization = originalFlag }()

	components := generateLargeComponentList(5000)
	filterStrings := []string{
		"./apps/*",
		"type=unit",
		"external=true",
		"./libs/*",
		"name=db",
		"./infra/*",
		"type=stack",
		"external=false",
		"./services/*",
		"name=api",
	}

	filters, err := filter.ParseFilterQueries(filterStrings)
	if err != nil {
		b.Fatal(err)
	}

	for b.Loop() {
		_, err := filters.Evaluate(components)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkMassiveUnionParallel benchmarks massive union with parallel evaluation
func BenchmarkMassiveUnionParallel(b *testing.B) {
	// Ensure parallel evaluation is enabled
	originalFlag := filter.DisableParallelization
	filter.DisableParallelization = false
	defer func() { filter.DisableParallelization = originalFlag }()

	components := generateLargeComponentList(5000)
	filterStrings := []string{
		"./apps/*",
		"type=unit",
		"external=true",
		"./libs/*",
		"name=db",
		"./infra/*",
		"type=stack",
		"external=false",
		"./services/*",
		"name=api",
	}

	filters, err := filter.ParseFilterQueries(filterStrings)
	if err != nil {
		b.Fatal(err)
	}

	for b.Loop() {
		_, err := filters.Evaluate(components)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// generateLargeComponentList creates a large list of components for benchmarking
func generateLargeComponentList(size int) []*component.Component {
	components := make([]*component.Component, size)

	for i := range size {
		var path string
		var kind component.Kind
		var external bool

		switch i % 4 {
		case 0:
			path = fmt.Sprintf("./apps/app%d", i)
			kind = component.Unit
			external = false
		case 1:
			path = fmt.Sprintf("./libs/lib%d", i)
			kind = component.Unit
			external = true
		case 2:
			path = fmt.Sprintf("./infra/infra%d", i)
			kind = component.Stack
			external = false
		case 3:
			path = fmt.Sprintf("./services/service%d", i)
			kind = component.Unit
			external = true
		}

		components[i] = &component.Component{
			Path:     path,
			Kind:     kind,
			External: external,
		}
	}

	return components
}
