# Catalog CLI Command End-to-End Testing

This document describes the comprehensive end-to-end testing implementation for the Terragrunt Catalog CLI command using `teatest` from Charm's experimental testing library.

## Overview

The Catalog CLI command provides a Terminal User Interface (TUI) for browsing and managing Terraform module catalogs. The testing implementation ensures that all interactive features work correctly and that the user experience is reliable.

## Testing Architecture

### Core Components Tested

1. **TUI Model State Management** - Verifies that the Bubble Tea model maintains correct state transitions
2. **Module Loading and Display** - Tests that modules are loaded from mock repositories and displayed correctly
3. **User Interactions** - Tests navigation, filtering, and other interactive features
4. **Golden File Testing** - Uses teatest's golden file functionality to ensure consistent output

### Test Files

- `cli/commands/catalog/tui/model_test.go` - Main TUI testing suite
- `cli/commands/catalog/catalog_test.go` - Integration tests for catalog command initialization
- `cli/commands/catalog/tui/testdata/TestTUIInitialOutput.golden` - Golden file for output testing

### Test Coverage

#### Core Functionality Tests

1. **TestTUIFinalModel** - Verifies that the TUI model reaches the correct final state after user interactions
2. **TestTUIInitialOutput** - Uses golden file testing to ensure consistent initial TUI output
3. **TestTUINavigationToModuleDetails** - Tests navigation from module list to detail view and back
4. **TestTUIModuleFiltering** - Tests the search/filter functionality for finding modules
5. **TestTUIWindowResize** - Verifies that the TUI handles terminal resize events gracefully

#### Integration Tests

1. **TestCatalogCommandInitialization** - Tests that the catalog service initializes correctly with mock repositories

## Key Features

### Mock Repository System

The tests use a sophisticated mocking system that creates temporary Git repositories with realistic structure:

```go
mockNewRepo := func(ctx context.Context, logger log.Logger, repoURL, path string, walkWithSymlinks, allowCAS bool) (*module.Repo, error) {
    // Creates temporary directory structure
    // Sets up mock .git configuration
    // Creates test modules with README.md and main.tf files
}
```

### Golden File Testing

The implementation follows teatest best practices:

- Uses `teatest.RequireEqualOutput(t, out)` for consistent output testing
- Includes `.gitattributes` file to handle golden files properly
- Golden files capture the exact TUI output for regression testing

### Parallel Test Execution

All tests use `t.Parallel()` for efficient execution and proper isolation.

### Robust Waiting Patterns

Tests use `teatest.WaitFor()` with appropriate timeouts and check intervals:

```go
teatest.WaitFor(t, tm.Output(), func(bts []byte) bool {
    return bytes.Contains(bts, []byte("List of Modules"))
}, teatest.WithCheckInterval(time.Millisecond*100), teatest.WithDuration(time.Second*2))
```

## Running the Tests

### Run All TUI Tests

```bash
go test -v ./cli/commands/catalog/tui/ -timeout 60s
```

### Run Specific Test

```bash
go test -v ./cli/commands/catalog/tui/ -run TestTUIInitialOutput
```

### Update Golden Files

```bash
go test -v ./cli/commands/catalog/tui/ -run TestTUIInitialOutput -update
```

### Run Integration Tests

```bash
go test -v ./cli/commands/catalog/ -run TestCatalogCommandInitialization
```

## Test Dependencies

The testing implementation leverages several key dependencies:

- `github.com/charmbracelet/x/exp/teatest` - Core TUI testing framework
- `github.com/charmbracelet/bubbletea` - The TUI framework being tested
- `github.com/stretchr/testify` - Assertion framework
- Mock catalog service with dependency injection for reliable testing

## Best Practices Implemented

1. **Consistent Color Profiles** - All tests work reliably in CI environments
2. **Proper Cleanup** - Uses `t.TempDir()` for automatic cleanup of test files
3. **Timeout Management** - Appropriate timeouts prevent hanging tests
4. **State Verification** - Tests verify both output and internal model state
5. **Mock Isolation** - Each test creates its own isolated mock environment

## Benefits

This testing implementation provides:

- **Confidence in UI Changes** - Any changes to the TUI will be caught by the golden file tests
- **Regression Prevention** - Comprehensive coverage prevents breaking existing functionality
- **Developer Experience** - Easy to add new tests following established patterns
- **CI/CD Integration** - Tests run reliably in automated environments
- **User Experience Validation** - Tests verify actual user workflows work correctly

## Future Enhancements

Potential areas for expansion:

1. **Error Scenario Testing** - Add tests for network failures, invalid repositories, etc.
2. **Performance Testing** - Add tests for large module catalogs
3. **Accessibility Testing** - Verify keyboard navigation and screen reader compatibility
4. **Cross-Platform Testing** - Ensure consistent behavior across different operating systems
