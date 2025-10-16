package filter

import "runtime"

// Concurrency configuration constants for filter evaluation optimization.
// These values are tuned for large-scale repos (500+ components) with focus on CI/CD throughput.
const (
	// StreamBufferSize is the default buffer size for streaming channels in intersection operations.
	// Increased for better throughput with large component lists.
	StreamBufferSize = 500

	// ParallelThreshold is the minimum number of components required to enable parallelization.
	// Raised significantly based on benchmark results showing overhead dominates benefits below this threshold.
	// Even at 20,000 components, parallelization is slower due to goroutine overhead.
	ParallelThreshold = 50000

	// MinFiltersForParallelUnion is the minimum number of positive filters required
	// to enable parallel evaluation in Filters.Evaluate() Phase 1.
	// Raised to avoid overhead for small filter sets.
	MinFiltersForParallelUnion = 5
)

// WorkerPoolSize returns the optimal number of workers for parallel component iteration.
// Uses CPU count but caps at reasonable maximum to avoid excessive context switching.
func WorkerPoolSize() int {
	cpuCount := runtime.NumCPU()
	if cpuCount > 8 {
		return 8 // Cap at 8 workers to avoid excessive context switching
	}
	return cpuCount
}

// StreamBufferSizeForComponents calculates optimal buffer size based on component count.
// Uses adaptive sizing to balance memory usage and throughput.
func StreamBufferSizeForComponents(numComponents int) int {
	if numComponents < 1000 {
		return 10 // Small buffer for small lists
	}
	if numComponents < 10000 {
		return min(numComponents/100, 50) // Medium buffer
	}
	return min(numComponents/200, 500) // Large buffer for big lists
}

// calculateBufferSize returns an adaptive buffer size based on input size.
// Uses min(inputSize/10, StreamBufferSize) to prevent memory explosion while maintaining throughput.
func calculateBufferSize(inputSize int) int {
	return StreamBufferSizeForComponents(inputSize)
}

// DisableParallelization is a global flag to force serial evaluation for testing.
// When true, all parallelization is disabled regardless of thresholds.
var DisableParallelization bool

// shouldUseParallelization returns true if parallelization should be used for the given input size.
func shouldUseParallelization(inputSize int) bool {
	if DisableParallelization {
		return false
	}
	return inputSize >= ParallelThreshold
}
