package filter

import (
	"context"
	"path/filepath"

	"github.com/gruntwork-io/terragrunt/internal/component"
	"golang.org/x/sync/errgroup"
)

// Evaluate evaluates an expression against a list of components and returns the filtered components.
func Evaluate(expr Expression, components []*component.Component) ([]*component.Component, error) {
	if expr == nil {
		return nil, NewEvaluationError("expression is nil")
	}

	return evaluate(expr, components)
}

// evaluate is the internal recursive evaluation function.
func evaluate(expr Expression, components []*component.Component) ([]*component.Component, error) {
	switch node := expr.(type) {
	case *PathFilter:
		return evaluatePathFilter(node, components)
	case *AttributeFilter:
		return evaluateAttributeFilter(node, components)
	case *PrefixExpression:
		return evaluatePrefixExpression(node, components)
	case *InfixExpression:
		return evaluateInfixExpression(node, components)
	default:
		return nil, NewEvaluationError("unknown expression type")
	}
}

// evaluatePathFilter evaluates a path filter using glob matching.
func evaluatePathFilter(filter *PathFilter, components []*component.Component) ([]*component.Component, error) {
	g, err := filter.CompileGlob()
	if err != nil {
		return nil, NewEvaluationErrorWithCause("failed to compile glob pattern: "+filter.Value, err)
	}

	return evaluateComponentsParallel(components, func(c *component.Component) bool {
		normalizedPath := filepath.ToSlash(c.Path)
		return g.Match(normalizedPath)
	}), nil
}

const (
	AttributeName     = "name"
	AttributeType     = "type"
	AttributeExternal = "external"

	AttributeTypeValueUnit  = string(component.Unit)
	AttributeTypeValueStack = string(component.Stack)

	AttributeExternalValueTrue  = "true"
	AttributeExternalValueFalse = "false"
)

// evaluateAttributeFilter evaluates an attribute filter.
func evaluateAttributeFilter(filter *AttributeFilter, components []*component.Component) ([]*component.Component, error) {
	switch filter.Key {
	case AttributeName:
		return evaluateComponentsParallel(components, func(c *component.Component) bool {
			return filepath.Base(c.Path) == filter.Value
		}), nil
	case AttributeType:
		switch filter.Value {
		case AttributeTypeValueUnit:
			return evaluateComponentsParallel(components, func(c *component.Component) bool {
				return c.Kind == component.Unit
			}), nil
		case AttributeTypeValueStack:
			return evaluateComponentsParallel(components, func(c *component.Component) bool {
				return c.Kind == component.Stack
			}), nil
		default:
			return nil, NewEvaluationError("invalid type value: " + filter.Value + " (expected 'unit' or 'stack')")
		}
	case AttributeExternal:
		switch filter.Value {
		case AttributeExternalValueTrue:
			return evaluateComponentsParallel(components, func(c *component.Component) bool {
				return c.External
			}), nil
		case AttributeExternalValueFalse:
			return evaluateComponentsParallel(components, func(c *component.Component) bool {
				return !c.External
			}), nil
		default:
			return nil, NewEvaluationError("invalid external value: " + filter.Value + " (expected 'true' or 'false')")
		}
	default:
		return nil, NewEvaluationError("unknown attribute key: " + filter.Key)
	}
}

// evaluatePrefixExpression evaluates a prefix expression (negation).
func evaluatePrefixExpression(expr *PrefixExpression, components []*component.Component) ([]*component.Component, error) {
	if expr.Operator != "!" {
		return nil, NewEvaluationError("unknown prefix operator: " + expr.Operator)
	}

	toExclude, err := evaluate(expr.Right, components)
	if err != nil {
		return nil, err
	}

	excludeSet := make(map[string]struct{}, len(toExclude))
	for _, c := range toExclude {
		excludeSet[c.Path] = struct{}{}
	}

	var result []*component.Component

	for _, c := range components {
		if _, ok := excludeSet[c.Path]; !ok {
			result = append(result, c)
		}
	}

	return result, nil
}

// evaluateInfixExpression evaluates an infix expression (intersection).
func evaluateInfixExpression(expr *InfixExpression, components []*component.Component) ([]*component.Component, error) {
	if expr.Operator != "|" {
		return nil, NewEvaluationError("unknown infix operator: " + expr.Operator)
	}

	if shouldUseParallelization(len(components)) {
		return evaluateInfixExpressionStreaming(expr, components)
	}

	leftResult, err := evaluate(expr.Left, components)
	if err != nil {
		return nil, err
	}

	rightResult, err := evaluate(expr.Right, leftResult)
	if err != nil {
		return nil, err
	}

	return rightResult, nil
}

// evaluateComponentsParallel evaluates components using a worker pool pattern for parallel processing.
// Returns components that match the predicate function.
func evaluateComponentsParallel(
	components []*component.Component,
	predicate func(*component.Component) bool,
) []*component.Component {
	if !shouldUseParallelization(len(components)) {
		var result []*component.Component
		for _, c := range components {
			if predicate(c) {
				result = append(result, c)
			}
		}
		return result
	}

	g, _ := errgroup.WithContext(context.Background())

	resultChan := make(chan *component.Component, len(components))

	numWorkers := WorkerPoolSize()
	chunkSize := len(components) / numWorkers
	if chunkSize < 200 {
		chunkSize = 200 // Higher minimum chunk size for efficiency
		numWorkers = len(components) / chunkSize
		if numWorkers == 0 {
			numWorkers = 1
		}
	}

	for i := 0; i < numWorkers; i++ {
		start := i * chunkSize
		end := start + chunkSize
		if i == numWorkers-1 {
			end = len(components)
		}

		if start >= len(components) {
			break
		}

		g.Go(func() error {
			for j := start; j < end && j < len(components); j++ {
				if predicate(components[j]) {
					select {
					case resultChan <- components[j]:
					default:
					}
				}
			}
			return nil
		})
	}

	go func() {
		g.Wait()
		close(resultChan)
	}()

	var result []*component.Component
	for comp := range resultChan {
		result = append(result, comp)
	}

	return result
}

// evaluateInfixExpressionStreaming implements a streaming pipeline for intersection operations.
// The left expression streams results to the right expression as they are found.
func evaluateInfixExpressionStreaming(expr *InfixExpression, components []*component.Component) ([]*component.Component, error) {
	bufferSize := calculateBufferSize(len(components))

	leftChan := make(chan *component.Component, bufferSize)
	rightChan := make(chan *component.Component, bufferSize)

	g, ctx := errgroup.WithContext(context.Background())

	g.Go(func() error {
		defer close(leftChan)
		return evaluateStreamingFromSlice(expr.Left, components, leftChan, ctx)
	})

	g.Go(func() error {
		defer close(rightChan)
		return evaluateStreaming(expr.Right, leftChan, rightChan, ctx)
	})

	var result []*component.Component
	g.Go(func() error {
		for c := range rightChan {
			result = append(result, c)
		}
		return nil
	})

	if err := g.Wait(); err != nil {
		return nil, err
	}

	return result, nil
}

// evaluateStreamingFromSlice evaluates an expression against a slice and streams results to a channel.
func evaluateStreamingFromSlice(expr Expression, components []*component.Component, outputChan chan<- *component.Component, ctx context.Context) error {
	switch node := expr.(type) {
	case *PathFilter:
		return evaluatePathFilterStreamingFromSlice(node, components, outputChan, ctx)
	case *AttributeFilter:
		return evaluateAttributeFilterStreamingFromSlice(node, components, outputChan, ctx)
	case *PrefixExpression:
		return evaluatePrefixExpressionStreamingFromSlice(node, components, outputChan, ctx)
	case *InfixExpression:
		return evaluateInfixExpressionStreamingFromSlice(node, components, outputChan, ctx)
	default:
		return NewEvaluationError("unknown expression type")
	}
}

// evaluatePathFilterStreamingFromSlice streams path filter results from a slice.
func evaluatePathFilterStreamingFromSlice(filter *PathFilter, components []*component.Component, outputChan chan<- *component.Component, ctx context.Context) error {
	g, err := filter.CompileGlob()
	if err != nil {
		return NewEvaluationErrorWithCause("failed to compile glob pattern: "+filter.Value, err)
	}

	for _, comp := range components {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			normalizedPath := filepath.ToSlash(comp.Path)
			if g.Match(normalizedPath) {
				select {
				case outputChan <- comp:
				case <-ctx.Done():
					return ctx.Err()
				}
			}
		}
	}
	return nil
}

// evaluateAttributeFilterStreamingFromSlice streams attribute filter results from a slice.
func evaluateAttributeFilterStreamingFromSlice(filter *AttributeFilter, components []*component.Component, outputChan chan<- *component.Component, ctx context.Context) error {
	for _, comp := range components {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			var matches bool
			switch filter.Key {
			case AttributeName:
				matches = filepath.Base(comp.Path) == filter.Value
			case AttributeType:
				switch filter.Value {
				case AttributeTypeValueUnit:
					matches = comp.Kind == component.Unit
				case AttributeTypeValueStack:
					matches = comp.Kind == component.Stack
				default:
					return NewEvaluationError("invalid type value: " + filter.Value + " (expected 'unit' or 'stack')")
				}
			case AttributeExternal:
				switch filter.Value {
				case AttributeExternalValueTrue:
					matches = comp.External
				case AttributeExternalValueFalse:
					matches = !comp.External
				default:
					return NewEvaluationError("invalid external value: " + filter.Value + " (expected 'true' or 'false')")
				}
			default:
				return NewEvaluationError("unknown attribute key: " + filter.Key)
			}

			if matches {
				select {
				case outputChan <- comp:
				case <-ctx.Done():
					return ctx.Err()
				}
			}
		}
	}
	return nil
}

// evaluatePrefixExpressionStreamingFromSlice streams negation results from a slice.
func evaluatePrefixExpressionStreamingFromSlice(expr *PrefixExpression, components []*component.Component, outputChan chan<- *component.Component, ctx context.Context) error {
	if expr.Operator != "!" {
		return NewEvaluationError("unknown prefix operator: " + expr.Operator)
	}

	// Evaluate the right side to get exclusions
	toExclude, err := evaluate(expr.Right, components)
	if err != nil {
		return err
	}

	// Create exclusion set
	excludeSet := make(map[string]struct{}, len(toExclude))
	for _, c := range toExclude {
		excludeSet[c.Path] = struct{}{}
	}

	// Stream non-excluded components
	for _, c := range components {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			if _, ok := excludeSet[c.Path]; !ok {
				select {
				case outputChan <- c:
				case <-ctx.Done():
					return ctx.Err()
				}
			}
		}
	}
	return nil
}

// evaluateInfixExpressionStreamingFromSlice handles nested infix expressions in streaming mode from a slice.
func evaluateInfixExpressionStreamingFromSlice(expr *InfixExpression, components []*component.Component, outputChan chan<- *component.Component, ctx context.Context) error {
	if expr.Operator != "|" {
		return NewEvaluationError("unknown infix operator: " + expr.Operator)
	}

	// Create intermediate channel for left -> right pipeline
	intermediateChan := make(chan *component.Component, calculateBufferSize(len(components)))

	g, ctx := errgroup.WithContext(ctx)

	// Start left side evaluator
	g.Go(func() error {
		defer close(intermediateChan)
		return evaluateStreamingFromSlice(expr.Left, components, intermediateChan, ctx)
	})

	// Start right side evaluator
	g.Go(func() error {
		return evaluateStreaming(expr.Right, intermediateChan, outputChan, ctx)
	})

	return g.Wait()
}

// evaluateStreaming evaluates an expression and streams results to an output channel.
// This is the core streaming evaluator that handles different expression types.
func evaluateStreaming(expr Expression, inputChan <-chan *component.Component, outputChan chan<- *component.Component, ctx context.Context) error {
	switch node := expr.(type) {
	case *PathFilter:
		return evaluatePathFilterStreaming(node, inputChan, outputChan, ctx)
	case *AttributeFilter:
		return evaluateAttributeFilterStreaming(node, inputChan, outputChan, ctx)
	case *PrefixExpression:
		return evaluatePrefixExpressionStreaming(node, inputChan, outputChan, ctx)
	case *InfixExpression:
		return evaluateInfixExpressionStreamingNested(node, inputChan, outputChan, ctx)
	default:
		return NewEvaluationError("unknown expression type")
	}
}

// evaluatePathFilterStreaming streams path filter results.
func evaluatePathFilterStreaming(filter *PathFilter, inputChan <-chan *component.Component, outputChan chan<- *component.Component, ctx context.Context) error {
	g, err := filter.CompileGlob()
	if err != nil {
		return NewEvaluationErrorWithCause("failed to compile glob pattern: "+filter.Value, err)
	}

	for {
		select {
		case comp, ok := <-inputChan:
			if !ok {
				return nil // input channel closed
			}

			normalizedPath := filepath.ToSlash(comp.Path)
			if g.Match(normalizedPath) {
				select {
				case outputChan <- comp:
				case <-ctx.Done():
					return ctx.Err()
				}
			}
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

// evaluateAttributeFilterStreaming streams attribute filter results.
func evaluateAttributeFilterStreaming(filter *AttributeFilter, inputChan <-chan *component.Component, outputChan chan<- *component.Component, ctx context.Context) error {
	for {
		select {
		case comp, ok := <-inputChan:
			if !ok {
				return nil // input channel closed
			}

			var matches bool
			switch filter.Key {
			case AttributeName:
				matches = filepath.Base(comp.Path) == filter.Value
			case AttributeType:
				switch filter.Value {
				case AttributeTypeValueUnit:
					matches = comp.Kind == component.Unit
				case AttributeTypeValueStack:
					matches = comp.Kind == component.Stack
				default:
					return NewEvaluationError("invalid type value: " + filter.Value + " (expected 'unit' or 'stack')")
				}
			case AttributeExternal:
				switch filter.Value {
				case AttributeExternalValueTrue:
					matches = comp.External
				case AttributeExternalValueFalse:
					matches = !comp.External
				default:
					return NewEvaluationError("invalid external value: " + filter.Value + " (expected 'true' or 'false')")
				}
			default:
				return NewEvaluationError("unknown attribute key: " + filter.Key)
			}

			if matches {
				select {
				case outputChan <- comp:
				case <-ctx.Done():
					return ctx.Err()
				}
			}
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

// evaluatePrefixExpressionStreaming streams negation results.
func evaluatePrefixExpressionStreaming(expr *PrefixExpression, inputChan <-chan *component.Component, outputChan chan<- *component.Component, ctx context.Context) error {
	if expr.Operator != "!" {
		return NewEvaluationError("unknown prefix operator: " + expr.Operator)
	}

	// For negation, we need to collect all components first, then exclude matches
	// This is a limitation of streaming - negation requires knowing all components
	var allComponents []*component.Component
	for {
		select {
		case component, ok := <-inputChan:
			if !ok {
				goto processExclusions
			}
			allComponents = append(allComponents, component)
		case <-ctx.Done():
			return ctx.Err()
		}
	}

processExclusions:

	// Evaluate the right side to get exclusions
	toExclude, err := evaluate(expr.Right, allComponents)
	if err != nil {
		return err
	}

	// Create exclusion set
	excludeSet := make(map[string]struct{}, len(toExclude))
	for _, c := range toExclude {
		excludeSet[c.Path] = struct{}{}
	}

	// Stream non-excluded components
	for _, c := range allComponents {
		if _, ok := excludeSet[c.Path]; !ok {
			select {
			case outputChan <- c:
			case <-ctx.Done():
				return ctx.Err()
			}
		}
	}

	return nil
}

// evaluateInfixExpressionStreamingNested handles nested infix expressions in streaming mode.
func evaluateInfixExpressionStreamingNested(expr *InfixExpression, inputChan <-chan *component.Component, outputChan chan<- *component.Component, ctx context.Context) error {
	if expr.Operator != "|" {
		return NewEvaluationError("unknown infix operator: " + expr.Operator)
	}

	// Create intermediate channel for left -> right pipeline
	intermediateChan := make(chan *component.Component, calculateBufferSize(100)) // reasonable default

	g, ctx := errgroup.WithContext(ctx)

	// Start left side evaluator
	g.Go(func() error {
		defer close(intermediateChan)
		return evaluateStreaming(expr.Left, inputChan, intermediateChan, ctx)
	})

	// Start right side evaluator
	g.Go(func() error {
		return evaluateStreaming(expr.Right, intermediateChan, outputChan, ctx)
	})

	return g.Wait()
}
