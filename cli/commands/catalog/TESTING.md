# Catalog CLI Command End-to-End Testing

This document describes the comprehensive end-to-end testing implementation for the Terragrunt Catalog CLI command using `teatest` from Charm's experimental testing library.

## Overview

Testing the TUI for the `catalog` command is a little tricky, as we can't conveniently have someone actually go in and test the TUI every time we make any change that could impact it. To make sure that we don't break the TUI, we take a layered approach to assuring the command works as expected.

1. The core logic used for the `catalog` command is actually handled in [services/catalog](../../../internal/services/catalog).

   This package can be tested in isolation with standard unit tests, and we minimize any logic done outside of it to reduce the surface area for testing of the TUI.

2. The TUI itself is tested using `teatest` from Charm's experimental testing library.

   This library provides a way to generate golden files that can be used to test the TUI to ensure that we don't encounter catastrophic regressions that would prevent loading of the TUI.

3. The `catalog` command initialization is tested in [catalog_test.go](catalog_test.go) to make sure we can setup the CLI command correctly to start up the TUI.

### Golden File Testing with teatest

- Uses `teatest.RequireEqualOutput(t, out)` for consistent output testing
- Includes `.gitattributes` file to handle golden files properly
- Golden files capture the exact TUI output for regression testing

### Waiting Patterns

Tests use `teatest.WaitFor()` with appropriate timeouts and check intervals:

```go
teatest.WaitFor(t, tm.Output(), func(bts []byte) bool {
    return bytes.Contains(bts, []byte("List of Modules"))
}, teatest.WithCheckInterval(time.Millisecond*100), teatest.WithDuration(time.Second*2))
```

## Running the Tests

### Update Golden Files

```bash
go test -v ./cli/commands/catalog/tui/ -run TestTUIInitialOutput -update
```
