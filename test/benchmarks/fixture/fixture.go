// Package fixture writes the unit layouts that the benchmarks run Terragrunt against.
package fixture

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"
)

// Shape names a unit layout that [Write] can produce.
type Shape string

const (
	// ShapeFlat is units that include the root configuration and are otherwise independent.
	ShapeFlat Shape = "flat"
	// ShapeDeps is [ShapeFlat] where every unit but the first reads an output of an earlier unit.
	// The units form a dependency tree in which each has up to [DepsFanOut] dependents.
	ShapeDeps Shape = "deps"
)

// DepsFanOut is how many units depend on each unit in [ShapeDeps]. It keeps the tree shallow enough
// that 1000 units are six levels deep, so a run stays mostly parallel.
const DepsFanOut = 4

// MaxUnits is the most units [Write] produces.
const MaxUnits = 10_000

// FileAge is how long before [Write] returns the modification time of every file it writes lies. Terragrunt re-reads
// a config whose modification time is within a few seconds of the read to catch a rewrite the time can't show, which
// files in a real repository are too old for.
const FileAge = time.Hour

const (
	dirPermissions  = 0o755
	filePermissions = 0o644

	rootConfig = `locals {
  project = "benchmark"
}

inputs = {
  project = local.project
}
`

	unitModule = `variable "project" {
  type = string
}

variable "name" {
  type = string
}

output "id" {
  value = "${var.project}-${var.name}"
}
`

	flatUnitConfig = `include "root" {
  path = find_in_parent_folders("root.hcl")
}

locals {
  name = %q
}

inputs = {
  name = local.name
}
`

	depsUnitConfig = `include "root" {
  path = find_in_parent_folders("root.hcl")
}

dependency "parent" {
  config_path = %q
}

locals {
  name = %q
}

inputs = {
  name = "${local.name}-${dependency.parent.outputs.id}"
}
`
)

// ErrUnknownShape is returned for a [Shape] that is not one of the declared constants.
var ErrUnknownShape = errors.New("unknown fixture shape")

// ErrUnitCount is returned for a unit count outside 1 to [MaxUnits].
var ErrUnitCount = errors.New("unit count out of range")

// Shapes returns every [Shape] that [Write] accepts.
func Shapes() []Shape {
	return []Shape{ShapeFlat, ShapeDeps}
}

// UnitName returns the directory name of the unit at index.
func UnitName(index int) string {
	return "unit-" + strconv.Itoa(index)
}

// Write creates dir and fills it with a root.hcl and the given number of units laid out as shape. Every file it writes
// is [FileAge] old.
//
// Returns [ErrUnknownShape] when shape is not a declared constant, and [ErrUnitCount] when units is
// outside 1 to [MaxUnits].
func Write(dir string, shape Shape, units int) error {
	if units < 1 || units > MaxUnits {
		return fmt.Errorf("%w: %d", ErrUnitCount, units)
	}

	if err := os.MkdirAll(dir, dirPermissions); err != nil {
		return err
	}

	modTime := time.Now().Add(-FileAge)

	if err := writeFile(filepath.Join(dir, "root.hcl"), rootConfig, modTime); err != nil {
		return err
	}

	for index := range units {
		config, err := unitConfig(shape, index)
		if err != nil {
			return err
		}

		unitDir := filepath.Join(dir, UnitName(index))

		if err := os.MkdirAll(unitDir, dirPermissions); err != nil {
			return err
		}

		if err := writeFile(filepath.Join(unitDir, "terragrunt.hcl"), config, modTime); err != nil {
			return err
		}

		if err := writeFile(filepath.Join(unitDir, "main.tf"), unitModule, modTime); err != nil {
			return err
		}
	}

	return nil
}

// writeFile writes content to the file at path and sets its access and modification times to modTime.
func writeFile(path, content string, modTime time.Time) error {
	if err := os.WriteFile(path, []byte(content), filePermissions); err != nil {
		return err
	}

	return os.Chtimes(path, modTime, modTime)
}

// unitConfig returns the terragrunt.hcl content of the unit at index.
func unitConfig(shape Shape, index int) (string, error) {
	switch shape {
	case ShapeFlat:
		return fmt.Sprintf(flatUnitConfig, UnitName(index)), nil
	case ShapeDeps:
		if index == 0 {
			return fmt.Sprintf(flatUnitConfig, UnitName(index)), nil
		}

		parent := "../" + UnitName((index-1)/DepsFanOut)

		return fmt.Sprintf(depsUnitConfig, parent, UnitName(index)), nil
	}

	return "", fmt.Errorf("%w: %q", ErrUnknownShape, shape)
}
