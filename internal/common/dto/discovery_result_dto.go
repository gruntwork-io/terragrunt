package dto

// DiscoveryResultDTO represents the complete output of a discovery operation.
// This DTO encapsulates all discovered units, stacks, their relationships,
// and any errors encountered during discovery.
type DiscoveryResultDTO struct {
	// Units contains all discovered Terragrunt units
	Units []*UnitDiscoveryDTO

	// Stacks contains all discovered Terragrunt stacks
	Stacks []*StackDiscoveryDTO

	// Relationships is a map from unit path to its dependency paths
	// This provides a quick lookup for dependency relationships without
	// needing to traverse the unit DTOs
	Relationships map[string][]string

	// Errors contains any non-fatal errors encountered during discovery
	// These are typically soft errors that don't prevent discovery from completing
	Errors []error

	// Metadata about the discovery operation
	Metadata *DiscoveryMetadata
}

// DiscoveryMetadata contains metadata about the discovery operation.
type DiscoveryMetadata struct {
	// WorkingDir is the directory where discovery was performed
	WorkingDir string

	// TotalUnitsFound is the total number of units found (including excluded)
	TotalUnitsFound int

	// TotalStacksFound is the total number of stacks found (including excluded)
	TotalStacksFound int

	// UnitsExcluded is the number of units excluded by filters
	UnitsExcluded int

	// StacksExcluded is the number of stacks excluded by filters
	StacksExcluded int

	// ParseErrors is the number of parse errors encountered
	ParseErrors int

	// DependenciesDiscovered is the number of dependency relationships discovered
	DependenciesDiscovered int
}

// NewDiscoveryResultDTO creates a new DiscoveryResultDTO with empty collections.
func NewDiscoveryResultDTO() *DiscoveryResultDTO {
	return &DiscoveryResultDTO{
		Units:         []*UnitDiscoveryDTO{},
		Stacks:        []*StackDiscoveryDTO{},
		Relationships: make(map[string][]string),
		Errors:        []error{},
		Metadata: &DiscoveryMetadata{
			TotalUnitsFound:        0,
			TotalStacksFound:       0,
			UnitsExcluded:          0,
			StacksExcluded:         0,
			ParseErrors:            0,
			DependenciesDiscovered: 0,
		},
	}
}

// AddUnit adds a unit to the result and returns the DTO for method chaining.
func (dto *DiscoveryResultDTO) AddUnit(unit *UnitDiscoveryDTO) *DiscoveryResultDTO {
	dto.Units = append(dto.Units, unit)
	dto.Metadata.TotalUnitsFound++
	if unit.FilterExcluded {
		dto.Metadata.UnitsExcluded++
	}
	if len(unit.ParseErrors) > 0 {
		dto.Metadata.ParseErrors += len(unit.ParseErrors)
	}
	return dto
}

// AddStack adds a stack to the result and returns the DTO for method chaining.
func (dto *DiscoveryResultDTO) AddStack(stack *StackDiscoveryDTO) *DiscoveryResultDTO {
	dto.Stacks = append(dto.Stacks, stack)
	dto.Metadata.TotalStacksFound++
	if stack.FilterExcluded {
		dto.Metadata.StacksExcluded++
	}
	if len(stack.ParseErrors) > 0 {
		dto.Metadata.ParseErrors += len(stack.ParseErrors)
	}
	return dto
}

// AddRelationship adds a dependency relationship and returns the DTO for method chaining.
func (dto *DiscoveryResultDTO) AddRelationship(unitPath string, dependencyPaths ...string) *DiscoveryResultDTO {
	existing := dto.Relationships[unitPath]
	dto.Relationships[unitPath] = append(existing, dependencyPaths...)
	dto.Metadata.DependenciesDiscovered += len(dependencyPaths)
	return dto
}

// AddError adds an error to the result and returns the DTO for method chaining.
func (dto *DiscoveryResultDTO) AddError(err error) *DiscoveryResultDTO {
	if err != nil {
		dto.Errors = append(dto.Errors, err)
	}
	return dto
}

// WithWorkingDir sets the working directory in metadata and returns the DTO for method chaining.
func (dto *DiscoveryResultDTO) WithWorkingDir(dir string) *DiscoveryResultDTO {
	dto.Metadata.WorkingDir = dir
	return dto
}

// GetIncludedUnits returns only the units that were not excluded by filters.
func (dto *DiscoveryResultDTO) GetIncludedUnits() []*UnitDiscoveryDTO {
	included := make([]*UnitDiscoveryDTO, 0, len(dto.Units)-dto.Metadata.UnitsExcluded)
	for _, unit := range dto.Units {
		if !unit.FilterExcluded {
			included = append(included, unit)
		}
	}
	return included
}

// GetIncludedStacks returns only the stacks that were not excluded by filters.
func (dto *DiscoveryResultDTO) GetIncludedStacks() []*StackDiscoveryDTO {
	included := make([]*StackDiscoveryDTO, 0, len(dto.Stacks)-dto.Metadata.StacksExcluded)
	for _, stack := range dto.Stacks {
		if !stack.FilterExcluded {
			included = append(included, stack)
		}
	}
	return included
}

// GetUnitByPath returns the unit DTO with the given path, or nil if not found.
func (dto *DiscoveryResultDTO) GetUnitByPath(path string) *UnitDiscoveryDTO {
	for _, unit := range dto.Units {
		if unit.Path == path {
			return unit
		}
	}
	return nil
}

// GetStackByPath returns the stack DTO with the given path, or nil if not found.
func (dto *DiscoveryResultDTO) GetStackByPath(path string) *StackDiscoveryDTO {
	for _, stack := range dto.Stacks {
		if stack.Path == path {
			return stack
		}
	}
	return nil
}

// HasErrors returns true if any errors were encountered during discovery.
func (dto *DiscoveryResultDTO) HasErrors() bool {
	return len(dto.Errors) > 0 || dto.Metadata.ParseErrors > 0
}
