package config

import (
	"fmt"
	"strings"

	"errors"
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

	var validationErrors []error

	if err := validateUnits(config.Units); err != nil {
		validationErrors = append(validationErrors, err)
	}

	if err := validateStacks(config.Stacks); err != nil {
		validationErrors = append(validationErrors, err)
	}

	return errors.Join(validationErrors...)
}

// validateUnits validates all units in the configuration
func validateUnits(units []*Unit) error {
	return validateConfigElementsGeneric(units, "unit", func(element any, i int) (string, string, string) {
		unit := element.(*Unit)
		return unit.Name, unit.Path, unit.Source
	})
}

// validateStacks validates all stacks in the configuration
func validateStacks(stacks []*Stack) error {
	return validateConfigElementsGeneric(stacks, "stack", func(element any, i int) (string, string, string) {
		stack := element.(*Stack)
		return stack.Name, stack.Path, stack.Source
	})
}

// validateConfigElementsGeneric is a generic function to validate configuration elements
// It takes a slice of elements, the element type name, and a function to extract name, path, and source from an element
func validateConfigElementsGeneric(elements any, elementType string, getValues func(element any, index int) (name, path, source string)) error {
	var validationErrors []error

	var slice []any

	// Convert the slice to a slice of interface{}
	switch v := elements.(type) {
	case []*Unit:
		slice = make([]any, len(v))
		for i, unit := range v {
			slice[i] = unit
		}
	case []*Stack:
		slice = make([]any, len(v))
		for i, stack := range v {
			slice[i] = stack
		}
	default:
		return errors.New("unknown element type")
	}

	names := make(map[string]bool, len(slice))
	paths := make(map[string]bool, len(slice))

	for i, element := range slice {
		if element == nil {
			validationErrors = append(validationErrors, fmt.Errorf("%s at index %d is nil", elementType, i))
			continue
		}

		name, path, source := getValues(element, i)
		name = strings.TrimSpace(name)
		path = strings.TrimSpace(path)
		source = strings.TrimSpace(source)

		// Validate name, source, and path
		if name == "" {
			validationErrors = append(validationErrors, fmt.Errorf("%s at index %d has empty name", elementType, i))
		}

		if source == "" {
			validationErrors = append(validationErrors, fmt.Errorf("%s '%s' has empty source", elementType, name))
		}

		if path == "" {
			validationErrors = append(validationErrors, fmt.Errorf("%s '%s' has empty path", elementType, name))
		}

		// Check for duplicates
		if names[name] {
			validationErrors = append(validationErrors, fmt.Errorf("duplicate %s name found: '%s'", elementType, name))
		}

		if paths[path] {
			validationErrors = append(validationErrors, fmt.Errorf("duplicate %s path found: '%s'", elementType, path))
		}

		// Save non-empty values for uniqueness check
		if name != "" {
			names[name] = true
		}

		if path != "" {
			paths[path] = true
		}
	}

	return errors.Join(validationErrors...)
}
