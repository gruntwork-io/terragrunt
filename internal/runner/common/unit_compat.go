package common

import "github.com/gruntwork-io/terragrunt/internal/component"

// Type aliases for backward compatibility.
// The actual Unit implementation has been moved to internal/component.
// These aliases allow existing code in the runner/common package to continue working.

type Unit = component.Unit
type Units = component.Units
type UnitsMap = component.UnitsMap
