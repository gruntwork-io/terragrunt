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
// - A unit and a stack shouldn't generate to the same path
//
// stackDir is the directory containing the stack file; it is used to compute the
// generated on-disk path of each unit and stack for the cross-kind collision check.
func ValidateStackConfig(config *StackConfigFile, stackDir string) error {
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

	if err := validateCrossKindPaths(config.Units, config.Stacks, stackDir); err != nil {
		validationErrors = append(validationErrors, err)
	}

	return errors.Join(validationErrors...)
}

// validateCrossKindPaths reports a generated path used by both a unit and a stack, since both
// components generate into the same on-disk directory and would collide. The comparison uses
// the normalized GeneratedPath (honoring no_dot_terragrunt_stack and path cleaning), not the raw
// path string, so it neither misses real collisions nor flags non-colliding raw strings.
// Within-kind duplicates are already reported by validateUnits and validateStacks.
func validateCrossKindPaths(units []*Unit, stacks []*Stack, stackDir string) error {
	unitGenPaths := make(map[string]struct{}, len(units))

	for _, u := range units {
		if u == nil {
			continue
		}

		if strings.TrimSpace(u.Path) == "" {
			continue
		}

		unitGenPaths[u.GeneratedPath(stackDir)] = struct{}{}
	}

	var validationErrors []error

	reported := make(map[string]struct{})

	for _, s := range stacks {
		if s == nil {
			continue
		}

		if strings.TrimSpace(s.Path) == "" {
			continue
		}

		genPath := s.GeneratedPath(stackDir)

		if _, collides := unitGenPaths[genPath]; !collides {
			continue
		}

		if _, seen := reported[genPath]; seen {
			continue
		}

		reported[genPath] = struct{}{}
		validationErrors = append(validationErrors, fmt.Errorf("duplicate path found across unit and stack: '%s'", genPath))
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

	// Convert the slice to a slice of interface{}.
	// A nil pointer is stored as an untyped nil so the element==nil guard below catches it,
	// otherwise a typed-nil pointer slips past the guard and getValues dereferences it.
	switch v := elements.(type) {
	case []*Unit:
		slice = make([]any, len(v))
		for i, unit := range v {
			if unit == nil {
				continue
			}

			slice[i] = unit
		}
	case []*Stack:
		slice = make([]any, len(v))
		for i, stack := range v {
			if stack == nil {
				continue
			}

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
