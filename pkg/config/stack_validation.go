package config

import (
	"path/filepath"
	"strings"

	"github.com/gruntwork-io/terragrunt/internal/errors"
)

// ValidateStackConfig validates a StackConfigFile instance according to the rules:
//   - Unit name, source, and path shouldn't be empty
//   - Unit names should be unique
//   - Stack name, source, and path shouldn't be empty
//   - Stack names should be unique
//   - No two components (unit or stack) may declare paths that overlap on disk
//     (identical paths or one being a directory ancestor of another). This
//     subsumes the older per-kind "duplicate path" check and additionally
//     catches cross-kind collisions and ancestor/descendant overlap. Such
//     overlaps race during concurrent generation: go-getter's RemoveAll(dst)
//     for subdir sources wipes a sibling's just-written content. Components
//     with no_dot_terragrunt_stack=true are validated as a separate namespace
//     because they emit outside the .terragrunt-stack/ directory.
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
		validationErrors = validationErrors.Append(err)
	}

	if err := validateStacks(config.Stacks); err != nil {
		validationErrors = validationErrors.Append(err)
	}

	if err := validatePathOverlaps(config.Units, config.Stacks); err != nil {
		validationErrors = validationErrors.Append(err)
	}

	return validationErrors.ErrorOrNil()
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
	validationErrors := &errors.MultiError{}

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

	for i, element := range slice {
		if element == nil {
			validationErrors = validationErrors.Append(errors.Errorf("%s at index %d is nil", elementType, i))
			continue
		}

		name, path, source := getValues(element, i)
		name = strings.TrimSpace(name)
		path = strings.TrimSpace(path)
		source = strings.TrimSpace(source)

		// Validate name, source, and path
		if name == "" {
			validationErrors = validationErrors.Append(errors.Errorf("%s at index %d has empty name", elementType, i))
		}

		if source == "" {
			validationErrors = validationErrors.Append(errors.Errorf("%s '%s' has empty source", elementType, name))
		}

		if path == "" {
			validationErrors = validationErrors.Append(errors.Errorf("%s '%s' has empty path", elementType, name))
		}

		// Check name uniqueness. Path collisions (exact or ancestor/descendant,
		// across kinds, with noStack awareness) are validated in
		// validatePathOverlaps so the diagnostic names both colliding blocks.
		if names[name] {
			validationErrors = validationErrors.Append(errors.Errorf("duplicate %s name found: '%s'", elementType, name))
		}

		if name != "" {
			names[name] = true
		}
	}

	return validationErrors.ErrorOrNil()
}

// pathOverlapCandidate is a flattened view of a unit or stack used for
// cross-kind path-overlap validation.
type pathOverlapCandidate struct {
	kind    string
	name    string
	rawPath string // the path as written in HCL, preserved for error messages
	clean   string // filepath.ToSlash(filepath.Clean(rawPath)) - the comparison key
	noStack bool   // no_dot_terragrunt_stack=true components live in a separate namespace
}

// validatePathOverlaps detects path collisions between any pair of components
// (unit-unit, stack-stack, or unit-stack) in the same stack file. A collision
// is either an identical normalized path or one path being a directory ancestor
// of another. Components with no_dot_terragrunt_stack=true are checked against
// each other only, since they emit outside the .terragrunt-stack/ directory
// and therefore cannot collide with regular components.
//
// Empty paths are skipped (those are reported separately by the per-kind checks)
// so that a misconfiguration is not double-reported.
func validatePathOverlaps(units []*Unit, stacks []*Stack) error {
	candidates := make([]pathOverlapCandidate, 0, len(units)+len(stacks))

	for _, u := range units {
		if u == nil {
			continue
		}

		path := strings.TrimSpace(u.Path)
		if path == "" {
			continue
		}

		candidates = append(candidates, pathOverlapCandidate{
			kind:    "unit",
			name:    strings.TrimSpace(u.Name),
			rawPath: u.Path,
			clean:   filepath.ToSlash(filepath.Clean(path)),
			noStack: u.NoStack != nil && *u.NoStack,
		})
	}

	for _, s := range stacks {
		if s == nil {
			continue
		}

		path := strings.TrimSpace(s.Path)
		if path == "" {
			continue
		}

		candidates = append(candidates, pathOverlapCandidate{
			kind:    "stack",
			name:    strings.TrimSpace(s.Name),
			rawPath: s.Path,
			clean:   filepath.ToSlash(filepath.Clean(path)),
			noStack: s.NoStack != nil && *s.NoStack,
		})
	}

	validationErrors := &errors.MultiError{}

	for i := 0; i < len(candidates); i++ {
		for j := i + 1; j < len(candidates); j++ {
			a, b := candidates[i], candidates[j]
			// Components in different namespaces (with vs without
			// no_dot_terragrunt_stack) cannot collide.
			if a.noStack != b.noStack {
				continue
			}

			if !pathsOverlap(a.clean, b.clean) {
				continue
			}

			validationErrors = validationErrors.Append(&OverlappingComponentPathsError{
				FirstKind:  a.kind,
				FirstName:  a.name,
				FirstPath:  a.rawPath,
				SecondKind: b.kind,
				SecondName: b.name,
				SecondPath: b.rawPath,
			})
		}
	}

	return validationErrors.ErrorOrNil()
}

// pathsOverlap reports whether two cleaned, forward-slash-normalized paths
// collide on disk: identical paths, or one being a directory ancestor of the
// other. Comparison uses a trailing slash to avoid false positives between
// names that share a textual prefix but not a directory ancestry (e.g.
// "app" and "app-other").
func pathsOverlap(a, b string) bool {
	if a == b {
		return true
	}

	return strings.HasPrefix(a, b+"/") || strings.HasPrefix(b, a+"/")
}
