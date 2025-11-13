package dto

import (
	"github.com/gruntwork-io/terragrunt/config"
	"github.com/gruntwork-io/terragrunt/internal/component"
)

// UnitDiscoveryDTO represents data transfer object for a discovered Terragrunt unit.
// This DTO is used to transfer discovery results from the discovery package to the runner package,
// avoiding tight coupling between these domains.
type UnitDiscoveryDTO struct {
	// Identity fields
	Path           string         // Absolute path to the unit directory
	Kind           component.Kind // "unit" or "stack"
	ConfigFilename string         // Name of the config file (e.g., "terragrunt.hcl")

	// Configuration data (parsed by discovery)
	Config *config.TerragruntConfig // Parsed Terragrunt configuration

	// Discovery metadata
	Reading          []string                    // Files read during parsing
	Sources          []string                    // Terraform sources from config
	DiscoveryContext *component.DiscoveryContext // Command context during discovery

	// Dependency information (as paths - not yet resolved to concrete objects)
	// These are extracted from the config and dependency graph during discovery
	DependencyPaths []string // Paths to units this unit depends on
	DependentPaths  []string // Paths to units that depend on this unit

	// Flags from discovery phase
	IsExternal     bool // True if unit is outside the working directory
	FilterExcluded bool // True if unit was excluded by filter rules

	// Optional metadata for runner
	ParseErrors   []error // Non-fatal parsing errors encountered during discovery
	RequiresApply bool    // True if this external dependency requires apply
}

// NewUnitDiscoveryDTO creates a new UnitDiscoveryDTO with the given path.
func NewUnitDiscoveryDTO(path string) *UnitDiscoveryDTO {
	return &UnitDiscoveryDTO{
		Path:            path,
		Kind:            component.UnitKind,
		Reading:         []string{},
		Sources:         []string{},
		DependencyPaths: []string{},
		DependentPaths:  []string{},
		ParseErrors:     []error{},
	}
}

// WithConfig sets the configuration and returns the DTO for method chaining.
func (dto *UnitDiscoveryDTO) WithConfig(cfg *config.TerragruntConfig) *UnitDiscoveryDTO {
	dto.Config = cfg
	return dto
}

// WithReading sets the reading files and returns the DTO for method chaining.
func (dto *UnitDiscoveryDTO) WithReading(reading ...string) *UnitDiscoveryDTO {
	dto.Reading = reading
	return dto
}

// WithSources sets the terraform sources and returns the DTO for method chaining.
func (dto *UnitDiscoveryDTO) WithSources(sources ...string) *UnitDiscoveryDTO {
	dto.Sources = sources
	return dto
}

// WithDiscoveryContext sets the discovery context and returns the DTO for method chaining.
func (dto *UnitDiscoveryDTO) WithDiscoveryContext(ctx *component.DiscoveryContext) *UnitDiscoveryDTO {
	dto.DiscoveryContext = ctx
	return dto
}

// WithDependencyPaths sets the dependency paths and returns the DTO for method chaining.
func (dto *UnitDiscoveryDTO) WithDependencyPaths(paths ...string) *UnitDiscoveryDTO {
	dto.DependencyPaths = paths
	return dto
}

// WithDependentPaths sets the dependent paths and returns the DTO for method chaining.
func (dto *UnitDiscoveryDTO) WithDependentPaths(paths ...string) *UnitDiscoveryDTO {
	dto.DependentPaths = paths
	return dto
}

// MarkExternal marks this unit as external and returns the DTO for method chaining.
func (dto *UnitDiscoveryDTO) MarkExternal() *UnitDiscoveryDTO {
	dto.IsExternal = true
	return dto
}

// MarkFilterExcluded marks this unit as excluded by filters and returns the DTO for method chaining.
func (dto *UnitDiscoveryDTO) MarkFilterExcluded() *UnitDiscoveryDTO {
	dto.FilterExcluded = true
	return dto
}

// MarkRequiresApply marks this unit as requiring apply and returns the DTO for method chaining.
func (dto *UnitDiscoveryDTO) MarkRequiresApply() *UnitDiscoveryDTO {
	dto.RequiresApply = true
	return dto
}

// AddParseError adds a parse error and returns the DTO for method chaining.
func (dto *UnitDiscoveryDTO) AddParseError(err error) *UnitDiscoveryDTO {
	if err != nil {
		dto.ParseErrors = append(dto.ParseErrors, err)
	}
	return dto
}
