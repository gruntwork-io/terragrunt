package dto

import (
	"github.com/gruntwork-io/terragrunt/config"
	"github.com/gruntwork-io/terragrunt/internal/component"
)

// StackDiscoveryDTO represents data transfer object for a discovered Terragrunt stack.
// This DTO is used to transfer discovery results for stack configurations
// from the discovery package to the runner package.
type StackDiscoveryDTO struct {
	// Identity fields
	Path           string // Absolute path to the stack directory
	ConfigFilename string // Name of the config file (e.g., "terragrunt.stack.hcl")

	// Configuration data (parsed by discovery)
	Config *config.StackConfig // Parsed stack configuration

	// Discovery metadata
	Reading          []string                    // Files read during parsing
	DiscoveryContext *component.DiscoveryContext // Command context during discovery

	// Flags from discovery phase
	IsExternal     bool // True if stack is outside the working directory
	FilterExcluded bool // True if stack was excluded by filter rules

	// Optional metadata
	ParseErrors []error // Non-fatal parsing errors encountered during discovery
}

// NewStackDiscoveryDTO creates a new StackDiscoveryDTO with the given path.
func NewStackDiscoveryDTO(path string) *StackDiscoveryDTO {
	return &StackDiscoveryDTO{
		Path:        path,
		Reading:     []string{},
		ParseErrors: []error{},
	}
}

// WithConfig sets the configuration and returns the DTO for method chaining.
func (dto *StackDiscoveryDTO) WithConfig(cfg *config.StackConfig) *StackDiscoveryDTO {
	dto.Config = cfg
	return dto
}

// WithReading sets the reading files and returns the DTO for method chaining.
func (dto *StackDiscoveryDTO) WithReading(reading ...string) *StackDiscoveryDTO {
	dto.Reading = reading
	return dto
}

// WithDiscoveryContext sets the discovery context and returns the DTO for method chaining.
func (dto *StackDiscoveryDTO) WithDiscoveryContext(ctx *component.DiscoveryContext) *StackDiscoveryDTO {
	dto.DiscoveryContext = ctx
	return dto
}

// MarkExternal marks this stack as external and returns the DTO for method chaining.
func (dto *StackDiscoveryDTO) MarkExternal() *StackDiscoveryDTO {
	dto.IsExternal = true
	return dto
}

// MarkFilterExcluded marks this stack as excluded by filters and returns the DTO for method chaining.
func (dto *StackDiscoveryDTO) MarkFilterExcluded() *StackDiscoveryDTO {
	dto.FilterExcluded = true
	return dto
}

// AddParseError adds a parse error and returns the DTO for method chaining.
func (dto *StackDiscoveryDTO) AddParseError(err error) *StackDiscoveryDTO {
	if err != nil {
		dto.ParseErrors = append(dto.ParseErrors, err)
	}
	return dto
}
