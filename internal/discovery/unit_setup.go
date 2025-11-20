package discovery

import (
	"context"
	"path/filepath"

	"github.com/gruntwork-io/terragrunt/config"
	"github.com/gruntwork-io/terragrunt/internal/component"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/telemetry"
	"github.com/gruntwork-io/terragrunt/util"
)

// telemetrySetupUnits wraps setupUnits in telemetry collection.
func (d *Discovery) telemetrySetupUnits(l log.Logger, discovered []component.Component) (component.Units, error) {
	var units component.Units

	err := telemetry.TelemeterFromContext(d.ctx).Collect(d.ctx, "setup_units", map[string]any{
		"working_dir": d.workingDir,
		"unit_count":  len(discovered),
	}, func(ctx context.Context) error {
		result, err := d.setupUnits(l, discovered)
		if err != nil {
			return err
		}

		units = result

		return nil
	})

	return units, err
}

// setupUnits constructs Units from discovery-parsed components without re-parsing,
// performing only the minimal parsing necessary to obtain missing fields (e.g., Terraform.source).
//
// This is the first stage of the unit resolution pipeline. It converts discovery components into
// Unit structs, preserving already-parsed configuration data to avoid redundant file I/O.
//
// The method:
//  1. Filters out non-units (e.g., stacks)
//  2. Skips units with parse errors from discovery
//  3. Determines the correct config file name (terragrunt.hcl or custom)
//  4. Resolves unit paths to canonical form
//  5. Checks if units should be excluded based on CLI flags (setting FlagExcluded=true)
//  6. Reuses parsed config from discovery (including TerraformSource and ErrorsBlock)
//  7. Sets up download directories for each unit
//  8. Skips units without Terraform source or TF files
//
// Units excluded at this stage have FlagExcluded=true and minimal configuration.
// They are still included in the Units slice for dependency resolution but won't be executed.
func (d *Discovery) setupUnits(l log.Logger, discovered []component.Component) (component.Units, error) {
	units := make(component.Units, 0, len(discovered))
	// Map from old unit path to new unit for dependency remapping
	oldToNew := make(map[string]*component.Unit)
	// Store old dependencies for later remapping
	oldDependencies := make(map[*component.Unit][]component.Component)

	for _, c := range discovered {
		// Only handle terraform units; skip stacks and anything else
		dUnit, ok := c.(*component.Unit)
		if !ok {
			continue
		}

		// Get the config that discovery already parsed
		terragruntConfig := dUnit.Config()
		if terragruntConfig == nil {
			// Skip configurations that discovery could not parse
			l.Warnf("Skipping unit at %s due to parse error", dUnit.Path())
			continue
		}

		// Determine the actual config file path
		terragruntConfigPath := dUnit.Path()
		if util.IsDir(terragruntConfigPath) {
			fname := d.determineConfigFilenameForUnit(dUnit.Path())
			terragruntConfigPath = filepath.Join(dUnit.Path(), fname)
		}

		unitPath, err := d.resolveUnitPath(terragruntConfigPath)
		if err != nil {
			return nil, err
		}

		// Prepare options with proper working dir
		l, opts, err := d.terragruntOptions.CloneWithConfigPath(l, terragruntConfigPath)
		if err != nil {
			return nil, err
		}

		opts.OriginalTerragruntConfigPath = terragruntConfigPath

		// Exclusion check - create a temporary unit for matching
		unitToExclude := component.NewUnit(unitPath)
		unitToExclude.SetTerragruntOptions(opts)
		unitToExclude.SetFlagExcluded(true)

		excludeFn := d.createPathMatcherFunc("exclude", opts, l)

		if excludeFn(unitToExclude) {
			units = append(units, unitToExclude)
			oldToNew[dUnit.Path()] = unitToExclude

			continue
		}

		// Determine effective source and setup download dir
		terragruntSource, err := config.GetTerragruntSourceForModule(d.terragruntOptions.Source, unitPath, terragruntConfig)
		if err != nil {
			return nil, err
		}

		opts.Source = terragruntSource

		// Update the config's source with the mapped source so that logging shows the correct URL
		if terragruntConfig.Terraform != nil && terragruntConfig.Terraform.Source != nil && terragruntSource != "" {
			terragruntConfig.Terraform.Source = &terragruntSource
		}

		if err = d.setupDownloadDir(terragruntConfigPath, opts, l); err != nil {
			return nil, err
		}

		// Preserve the external flag from discovery component
		isExternal := dUnit.External()

		// NOTE: We used to skip units without terraform configurations here, but this breaks
		// discovery commands (find, list) that need to show ALL units regardless of whether
		// they have terraform configs. The runner is responsible for filtering units that
		// can't be executed, not discovery.
		//
		// The validation is intentionally removed to restore the original behavior where
		// discovery discovers everything, and filtering happens later in the pipeline.

		unit := component.NewUnit(unitPath)
		unit.StoreConfig(terragruntConfig)
		unit.SetTerragruntOptions(opts)
		unit.SetReading(dUnit.Reading()...)
		unit.SetDiscoveryContext(dUnit.DiscoveryContext())

		if isExternal {
			unit.SetExternal()
		}

		units = append(units, unit)
		// Store mapping and dependencies for later remapping
		oldToNew[dUnit.Path()] = unit
		if len(dUnit.Dependencies()) > 0 {
			oldDependencies[unit] = dUnit.Dependencies()
		}
	}

	// Remap dependencies to point to the new units
	for unit, deps := range oldDependencies {
		for _, dep := range deps {
			// Look up the new unit for this dependency
			if newDep, ok := oldToNew[dep.Path()]; ok {
				unit.AddDependency(newDep)
			} else {
				// If we don't have a new unit for this dependency (e.g., external),
				// keep the original dependency
				unit.AddDependency(dep)
			}
		}
	}

	return units, nil
}

// resolveUnitPath converts a Terragrunt configuration file path to its corresponding unit path.
// Returns the canonical path of the directory containing the config file.
func (d *Discovery) resolveUnitPath(terragruntConfigPath string) (string, error) {
	return util.CanonicalPath(filepath.Dir(terragruntConfigPath), ".")
}

// setupDownloadDir sets the unit's download directory.
// If the stack uses the default dir, compute a per-unit dir; otherwise use the stack's setting.
func (d *Discovery) setupDownloadDir(terragruntConfigPath string, opts *options.TerragruntOptions, l log.Logger) error {
	_, defaultDownloadDir, err := options.DefaultWorkingAndDownloadDirs(d.terragruntOptions.TerragruntConfigPath)
	if err != nil {
		return err
	}

	if d.terragruntOptions.DownloadDir == defaultDownloadDir {
		_, downloadDir, err := options.DefaultWorkingAndDownloadDirs(terragruntConfigPath)
		if err != nil {
			return err
		}

		l.Debugf("Setting download directory for unit %s to %s", filepath.Dir(opts.TerragruntConfigPath), downloadDir)
		opts.DownloadDir = downloadDir
	}

	return nil
}

// determineTerragruntConfigFilename returns the config filename to use.
// If a file path is explicitly set, it uses its basename; otherwise, "terragrunt.hcl".
func (d *Discovery) determineTerragruntConfigFilename() string {
	fname := config.DefaultTerragruntConfigPath
	if d.terragruntOptions.TerragruntConfigPath != "" && !util.IsDir(d.terragruntOptions.TerragruntConfigPath) {
		fname = filepath.Base(d.terragruntOptions.TerragruntConfigPath)
	}

	return fname
}

// determineConfigFilenameForUnit determines which config file exists in the given unit directory.
// It checks for custom config filenames first, then falls back to the default.
func (d *Discovery) determineConfigFilenameForUnit(unitDir string) string {
	// Get the list of config filenames to check (custom or default)
	filenames := d.configFilenames
	if len(filenames) == 0 {
		// Fall back to checking TerragruntConfigPath for custom filename
		return d.determineTerragruntConfigFilename()
	}

	// Check each custom config filename to see which one exists in this directory
	for _, fname := range filenames {
		configPath := filepath.Join(unitDir, fname)
		if util.FileExists(configPath) {
			return fname
		}
	}

	// If none of the custom filenames exist, return the first one as default
	return filenames[0]
}
