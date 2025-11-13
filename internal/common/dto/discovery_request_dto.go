package dto

import (
	"github.com/gruntwork-io/terragrunt/config/hclparse"
	"github.com/gruntwork-io/terragrunt/internal/component"
	"github.com/gruntwork-io/terragrunt/internal/filter"
)

// DiscoveryRequestDTO represents the request parameters for discovery operations.
// This DTO encapsulates all configuration needed to perform discovery,
// allowing the discovery package to be invoked without tight coupling to CLI options.
type DiscoveryRequestDTO struct {
	// WorkingDir is the directory to search for Terragrunt configurations
	WorkingDir string

	// DiscoveryContext provides the command context for discovery
	DiscoveryContext *component.DiscoveryContext

	// ConfigFilenames is the list of config filenames to discover (e.g., ["terragrunt.hcl"])
	// If nil or empty, defaults will be used
	ConfigFilenames []string

	// IncludeDirs is a list of directory patterns to include in discovery (glob patterns)
	// Used for strict include mode
	IncludeDirs []string

	// ExcludeDirs is a list of directory patterns to exclude from discovery (glob patterns)
	ExcludeDirs []string

	// Filters contains filter queries for component selection
	Filters filter.Filters

	// ParserOptions are custom HCL parser options to use during discovery
	ParserOptions []hclparse.Option

	// Discovery behavior flags
	DiscoverDependencies bool // Whether to discover dependencies
	ExcludeByDefault     bool // Whether to exclude configurations by default
	NoHidden             bool // Whether to detect configurations in hidden directories
	RequiresParse        bool // Whether discovery requires parsing configurations
	StrictInclude        bool // Whether to use strict include mode
	ParseExclude         bool // Whether to parse exclude configurations
	ParseInclude         bool // Whether to parse include configurations

	// Performance tuning
	NumWorkers         int // Number of concurrent workers (0 = use default)
	MaxDependencyDepth int // Maximum depth of dependency tree (0 = use default)

	// Sort determines the sort order of discovered configurations
	Sort string // "name", "path", etc.
}

// NewDiscoveryRequestDTO creates a new DiscoveryRequestDTO with sensible defaults.
func NewDiscoveryRequestDTO(workingDir string) *DiscoveryRequestDTO {
	return &DiscoveryRequestDTO{
		WorkingDir:           workingDir,
		ConfigFilenames:      nil, // Use discovery defaults
		IncludeDirs:          []string{},
		ExcludeDirs:          []string{},
		Filters:              filter.Filters{},
		ParserOptions:        []hclparse.Option{},
		DiscoverDependencies: true,
		ExcludeByDefault:     false,
		NoHidden:             false,
		RequiresParse:        false,
		StrictInclude:        false,
		ParseExclude:         false,
		ParseInclude:         false,
		NumWorkers:           0, // Use default
		MaxDependencyDepth:   0, // Use default
		Sort:                 "",
	}
}

// WithDiscoveryContext sets the discovery context and returns the DTO for method chaining.
func (dto *DiscoveryRequestDTO) WithDiscoveryContext(ctx *component.DiscoveryContext) *DiscoveryRequestDTO {
	dto.DiscoveryContext = ctx
	return dto
}

// WithConfigFilenames sets the config filenames and returns the DTO for method chaining.
func (dto *DiscoveryRequestDTO) WithConfigFilenames(filenames ...string) *DiscoveryRequestDTO {
	dto.ConfigFilenames = filenames
	return dto
}

// WithIncludeDirs sets the include directories and returns the DTO for method chaining.
func (dto *DiscoveryRequestDTO) WithIncludeDirs(dirs ...string) *DiscoveryRequestDTO {
	dto.IncludeDirs = dirs
	return dto
}

// WithExcludeDirs sets the exclude directories and returns the DTO for method chaining.
func (dto *DiscoveryRequestDTO) WithExcludeDirs(dirs ...string) *DiscoveryRequestDTO {
	dto.ExcludeDirs = dirs
	return dto
}

// WithFilters sets the filters and returns the DTO for method chaining.
func (dto *DiscoveryRequestDTO) WithFilters(filters filter.Filters) *DiscoveryRequestDTO {
	dto.Filters = filters
	return dto
}

// WithParserOptions sets the parser options and returns the DTO for method chaining.
func (dto *DiscoveryRequestDTO) WithParserOptions(opts ...hclparse.Option) *DiscoveryRequestDTO {
	dto.ParserOptions = opts
	return dto
}

// WithDiscoverDependencies sets whether to discover dependencies and returns the DTO for method chaining.
func (dto *DiscoveryRequestDTO) WithDiscoverDependencies(discover bool) *DiscoveryRequestDTO {
	dto.DiscoverDependencies = discover
	return dto
}

// WithNumWorkers sets the number of workers and returns the DTO for method chaining.
func (dto *DiscoveryRequestDTO) WithNumWorkers(n int) *DiscoveryRequestDTO {
	dto.NumWorkers = n
	return dto
}

// WithMaxDependencyDepth sets the max dependency depth and returns the DTO for method chaining.
func (dto *DiscoveryRequestDTO) WithMaxDependencyDepth(depth int) *DiscoveryRequestDTO {
	dto.MaxDependencyDepth = depth
	return dto
}

// WithSort sets the sort order and returns the DTO for method chaining.
func (dto *DiscoveryRequestDTO) WithSort(sort string) *DiscoveryRequestDTO {
	dto.Sort = sort
	return dto
}

// EnableStrictInclude enables strict include mode and returns the DTO for method chaining.
func (dto *DiscoveryRequestDTO) EnableStrictInclude() *DiscoveryRequestDTO {
	dto.StrictInclude = true
	return dto
}

// EnableExcludeByDefault enables exclude by default and returns the DTO for method chaining.
func (dto *DiscoveryRequestDTO) EnableExcludeByDefault() *DiscoveryRequestDTO {
	dto.ExcludeByDefault = true
	return dto
}

// EnableNoHidden enables no hidden detection and returns the DTO for method chaining.
func (dto *DiscoveryRequestDTO) EnableNoHidden() *DiscoveryRequestDTO {
	dto.NoHidden = true
	return dto
}

// EnableRequiresParse enables parse requirement and returns the DTO for method chaining.
func (dto *DiscoveryRequestDTO) EnableRequiresParse() *DiscoveryRequestDTO {
	dto.RequiresParse = true
	return dto
}

// EnableParseExclude enables parse exclude and returns the DTO for method chaining.
func (dto *DiscoveryRequestDTO) EnableParseExclude() *DiscoveryRequestDTO {
	dto.ParseExclude = true
	return dto
}

// EnableParseInclude enables parse include and returns the DTO for method chaining.
func (dto *DiscoveryRequestDTO) EnableParseInclude() *DiscoveryRequestDTO {
	dto.ParseInclude = true
	return dto
}
