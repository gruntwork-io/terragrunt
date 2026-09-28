package config

import (
	"errors"
	"strings"
)

// ValidateStackConfig validates the components a StackConfigFile resolved to, according to
// the rules:
// - Unit name, source, and path shouldn't be empty
// - Unit names should be unique
// - Units shouldn't generate to duplicate paths
// - Stack name, source, and path shouldn't be empty
// - Stack names should be unique
// - Stacks shouldn't generate to duplicate paths
// - A unit and a stack shouldn't generate to the same path
//
// Components are validated after expansion. The names compared are keyed addresses, and the
// paths compared are the directories each component generates into under stackDir.
//
// Resolving to no components at all is valid here: a file that declares blocks which all
// expand to zero elements or set enabled to false has nothing to generate, and
// [ParseStackConfig] is what rejects a file that declares no blocks in the first place.
func ValidateStackConfig(config *StackConfigFile, stackDir string) error {
	if config == nil {
		return errors.New("stack config cannot be nil")
	}

	units := unitViews(config.Units, stackDir)
	stacks := stackViews(config.Stacks, stackDir)

	return errors.Join(
		validateComponents(units, ComponentKindUnit),
		validateComponents(stacks, ComponentKindStack),
		validateCrossKindPaths(units, stacks),
	)
}

// componentView is how stack validation reads a unit or stack. The two kinds carry the same
// fields and obey the same uniqueness rules, so one implementation checks both.
//
// [decodeUnitBlocks] and [decodeStackBlocks] construct every element they return, and the
// autoinclude merge only passes those values along, so the slices read here hold no nil
// components.
type componentView struct {
	label         string
	address       string
	source        string
	generatedPath string
}

func unitViews(units []*Unit, stackDir string) []componentView {
	views := make([]componentView, len(units))

	for i, u := range units {
		label := strings.TrimSpace(u.Name)

		views[i] = componentView{
			label:         label,
			address:       componentAddress(label, u.Expansion),
			source:        strings.TrimSpace(u.Source),
			generatedPath: generatedPathOrEmpty(u.Path, u.GeneratedPath(stackDir)),
		}
	}

	return views
}

func stackViews(stacks []*Stack, stackDir string) []componentView {
	views := make([]componentView, len(stacks))

	for i, s := range stacks {
		label := strings.TrimSpace(s.Name)

		views[i] = componentView{
			label:         label,
			address:       componentAddress(label, s.Expansion),
			source:        strings.TrimSpace(s.Source),
			generatedPath: generatedPathOrEmpty(s.Path, s.GeneratedPath(stackDir)),
		}
	}

	return views
}

// generatedPathOrEmpty returns the empty string for a component that declared no path.
// Joining an empty path lands on the stack directory itself, so two components that both
// omit path would otherwise compare equal and be reported as colliding.
func generatedPathOrEmpty(rawPath, generatedPath string) string {
	if strings.TrimSpace(rawPath) == "" {
		return ""
	}

	return generatedPath
}

// validateComponents reports the components of one kind that are missing a required field
// or that collide with each other on address or generated path.
func validateComponents(views []componentView, kind ComponentKind) error {
	var validationErrors []error

	addresses := make(map[string]struct{}, len(views))
	paths := make(map[string]struct{}, len(views))

	for i, view := range views {
		// Ordered rather than ranged over a map so a component missing more than one field
		// reports them in the same order every run.
		for _, required := range []struct {
			field string
			value string
		}{
			{field: "name", value: view.label},
			{field: "source", value: view.source},
			{field: "path", value: view.generatedPath},
		} {
			if required.value != "" {
				continue
			}

			validationErrors = append(validationErrors, ComponentFieldEmptyError{
				Kind:  kind,
				Field: required.field,
				Name:  view.address,
				Index: i,
			})
		}

		if view.label != "" {
			if _, duplicate := addresses[view.address]; duplicate {
				validationErrors = append(
					validationErrors,
					DuplicateComponentNameError{Kind: kind, Name: view.address},
				)
			}

			addresses[view.address] = struct{}{}
		}

		if view.generatedPath != "" {
			if _, duplicate := paths[view.generatedPath]; duplicate {
				validationErrors = append(
					validationErrors,
					DuplicateComponentPathError{Kind: kind, Path: view.generatedPath},
				)
			}

			paths[view.generatedPath] = struct{}{}
		}
	}

	return errors.Join(validationErrors...)
}

// validateCrossKindPaths reports a generated path used by both a unit and a stack, since both
// components generate into the same on-disk directory and would collide. Within-kind
// duplicates are already reported by [validateComponents].
func validateCrossKindPaths(units, stacks []componentView) error {
	unitPaths := make(map[string]struct{}, len(units))

	for _, u := range units {
		if u.generatedPath == "" {
			continue
		}

		unitPaths[u.generatedPath] = struct{}{}
	}

	var validationErrors []error

	reported := make(map[string]struct{})

	for _, s := range stacks {
		if s.generatedPath == "" {
			continue
		}

		if _, collides := unitPaths[s.generatedPath]; !collides {
			continue
		}

		if _, seen := reported[s.generatedPath]; seen {
			continue
		}

		reported[s.generatedPath] = struct{}{}
		validationErrors = append(
			validationErrors,
			ComponentPathCollisionError{Path: s.generatedPath},
		)
	}

	return errors.Join(validationErrors...)
}
