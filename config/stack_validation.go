package config

import (
	"strings"

	"github.com/gruntwork-io/terragrunt/internal/errors"
)

// ValidateStackConfig validates a StackConfigFile instance according to the rules:
// - Unit name, source, and path shouldn't be empty
// - Unit names should be unique
// - Units shouldn't have duplicate paths
// - Stack name, source, and path shouldn't be empty
// - Stack names should be unique
// - Stack shouldn't have duplicate paths
func ValidateStackConfig(config *StackConfigFile) error {
	if config == nil {
		return errors.New("stack config cannot be nil")
	}

	// Check if we have any units or stacks
	if len(config.Units) == 0 && len(config.Stacks) == 0 {
		return errors.New("stack config must contain at least one unit or stack")
	}

	validationErrors := &errors.MultiError{}
	if err := validateUnits(config.Units); err != nil {
		validationErrors.Append(err)
	}
	if err := validateStacks(config.Stacks); err != nil {
		validationErrors.Append(err)
	}
	return validationErrors.ErrorOrNil()
}

// validateUnits validates all units in the configuration
func validateUnits(units []*Unit) error {
	if len(units) == 0 {
		return nil
	}
	validationErrors := &errors.MultiError{}
	// Pre-allocate maps with known capacity to avoid resizing
	names := make(map[string]bool, len(units))
	paths := make(map[string]bool, len(units))

	for i, unit := range units {
		if unit == nil {
			validationErrors.Append(errors.Errorf("unit at index %d is nil", i))
			continue
		}

		name := strings.TrimSpace(unit.Name)
		path := strings.TrimSpace(unit.Path)
		source := strings.TrimSpace(unit.Source)

		// Validate name
		if name == "" {
			validationErrors.Append(errors.Errorf("unit at index %d has empty name", i))
		}

		// Validate source
		if source == "" {
			validationErrors.Append(errors.Errorf("unit '%s' has empty source", unit.Name))
		}

		// Validate path
		if path == "" {
			validationErrors.Append(errors.Errorf("unit '%s' has empty path", unit.Name))
		}

		// Check for duplicates
		if names[name] {
			validationErrors.Append(errors.Errorf("duplicate unit name found: '%s'", unit.Name))
		}

		if paths[path] {
			validationErrors.Append(errors.Errorf("duplicate unit path found: '%s'", unit.Path))
		}

		// Save non-empty values for uniqueness check
		if name != "" {
			names[name] = true
		}

		if path != "" {
			paths[path] = true
		}
	}
	return validationErrors.ErrorOrNil()
}

// validateStacks validates all stacks in the configuration
func validateStacks(stacks []*Stack) error {
	if len(stacks) == 0 {
		return nil
	}
	validationErrors := &errors.MultiError{}
	// Pre-allocate maps with known capacity to avoid resizing
	names := make(map[string]bool, len(stacks))
	paths := make(map[string]bool, len(stacks))

	for i, stack := range stacks {
		if stack == nil {
			validationErrors.Append(errors.Errorf("stack at index %d is nil", i))
			continue
		}

		name := strings.TrimSpace(stack.Name)
		path := strings.TrimSpace(stack.Path)
		source := strings.TrimSpace(stack.Source)

		// Validate name
		if name == "" {
			validationErrors.Append(errors.Errorf("stack at index %d has empty name", i))
		}

		// Validate source
		if source == "" {
			validationErrors.Append(errors.Errorf("stack '%s' has empty source", stack.Name))
		}

		// Validate path
		if path == "" {
			validationErrors.Append(errors.Errorf("stack '%s' has empty path", stack.Name))
		}

		// Check for duplicates
		if names[name] {
			validationErrors.Append(errors.Errorf("duplicate stack name found: '%s'", stack.Name))
		}

		if paths[path] {
			validationErrors.Append(errors.Errorf("duplicate stack path found: '%s'", stack.Path))
		}

		// Save non-empty values for uniqueness check
		if name != "" {
			names[name] = true
		}

		if path != "" {
			paths[path] = true
		}
	}
	return validationErrors.ErrorOrNil()
}
