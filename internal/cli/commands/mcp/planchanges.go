package mcp

import (
	"errors"
	"fmt"

	"github.com/gruntwork-io/terragrunt/internal/tf"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
)

// annotatePlanChanges fills in the plan JSON and change counts of each unit
// result, and returns a note for each unit whose plan JSON could not be read.
// jsonFiles maps each unit's path to its plan JSON file.
func annotatePlanChanges(
	fsys vfs.FS,
	units []runAllUnitResult,
	jsonFiles map[string]string,
) []string {
	var notes []string

	for i := range units {
		jsonFile, ok := jsonFiles[units[i].absPath]
		if !ok || !vfs.Exists(fsys, jsonFile) {
			continue
		}

		changes, err := readPlanChanges(fsys, jsonFile)
		if err != nil {
			notes = append(
				notes,
				fmt.Sprintf("plan JSON for %s could not be read: %v", units[i].Path, err),
			)

			continue
		}

		units[i].PlanJSON = jsonFile
		units[i].Changes = changes
	}

	return notes
}

// readPlanChanges counts the changes in the plan JSON at path.
func readPlanChanges(fsys vfs.FS, path string) (*tf.PlanChanges, error) {
	file, err := fsys.Open(path)
	if err != nil {
		return nil, err
	}

	changes, decodeErr := tf.DecodePlanChanges(file)

	if err := errors.Join(decodeErr, file.Close()); err != nil {
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}

	return &changes, nil
}
