package dto

import "time"

// ExecutionResultDTO represents the result of executing a single unit.
// This DTO allows runner results to be passed back to other layers
// (e.g., for reporting, caching, or analysis).
type ExecutionResultDTO struct {
	// UnitPath is the path to the unit that was executed
	UnitPath string

	// Status indicates the execution outcome
	Status ExecutionStatus

	// StartTime is when execution started
	StartTime time.Time

	// EndTime is when execution completed
	EndTime time.Time

	// Output contains the stdout/stderr from execution
	Output string

	// Error is the error encountered during execution (if any)
	Error error

	// ExitCode is the exit code from the execution
	ExitCode int

	// Metadata contains additional execution metadata
	Metadata map[string]interface{}
}

// ExecutionStatus represents the status of a unit execution.
type ExecutionStatus string

const (
	// ExecutionStatusPending indicates the unit has not started executing
	ExecutionStatusPending ExecutionStatus = "pending"

	// ExecutionStatusRunning indicates the unit is currently executing
	ExecutionStatusRunning ExecutionStatus = "running"

	// ExecutionStatusSuccess indicates the unit executed successfully
	ExecutionStatusSuccess ExecutionStatus = "success"

	// ExecutionStatusFailed indicates the unit execution failed
	ExecutionStatusFailed ExecutionStatus = "failed"

	// ExecutionStatusSkipped indicates the unit was skipped
	ExecutionStatusSkipped ExecutionStatus = "skipped"

	// ExecutionStatusCancelled indicates the unit execution was cancelled
	ExecutionStatusCancelled ExecutionStatus = "cancelled"
)

// NewExecutionResultDTO creates a new ExecutionResultDTO for the given unit path.
func NewExecutionResultDTO(unitPath string) *ExecutionResultDTO {
	return &ExecutionResultDTO{
		UnitPath:  unitPath,
		Status:    ExecutionStatusPending,
		Metadata:  make(map[string]interface{}),
		StartTime: time.Time{},
		EndTime:   time.Time{},
	}
}

// WithStatus sets the status and returns the DTO for method chaining.
func (dto *ExecutionResultDTO) WithStatus(status ExecutionStatus) *ExecutionResultDTO {
	dto.Status = status
	return dto
}

// WithStartTime sets the start time and returns the DTO for method chaining.
func (dto *ExecutionResultDTO) WithStartTime(t time.Time) *ExecutionResultDTO {
	dto.StartTime = t
	return dto
}

// WithEndTime sets the end time and returns the DTO for method chaining.
func (dto *ExecutionResultDTO) WithEndTime(t time.Time) *ExecutionResultDTO {
	dto.EndTime = t
	return dto
}

// WithOutput sets the output and returns the DTO for method chaining.
func (dto *ExecutionResultDTO) WithOutput(output string) *ExecutionResultDTO {
	dto.Output = output
	return dto
}

// WithError sets the error and returns the DTO for method chaining.
func (dto *ExecutionResultDTO) WithError(err error) *ExecutionResultDTO {
	dto.Error = err
	if err != nil && dto.Status == ExecutionStatusPending {
		dto.Status = ExecutionStatusFailed
	}
	return dto
}

// WithExitCode sets the exit code and returns the DTO for method chaining.
func (dto *ExecutionResultDTO) WithExitCode(code int) *ExecutionResultDTO {
	dto.ExitCode = code
	return dto
}

// AddMetadata adds a metadata key-value pair and returns the DTO for method chaining.
func (dto *ExecutionResultDTO) AddMetadata(key string, value interface{}) *ExecutionResultDTO {
	dto.Metadata[key] = value
	return dto
}

// Duration returns the duration of the execution.
func (dto *ExecutionResultDTO) Duration() time.Duration {
	if dto.StartTime.IsZero() || dto.EndTime.IsZero() {
		return 0
	}
	return dto.EndTime.Sub(dto.StartTime)
}

// IsSuccess returns true if the execution was successful.
func (dto *ExecutionResultDTO) IsSuccess() bool {
	return dto.Status == ExecutionStatusSuccess
}

// IsFailed returns true if the execution failed.
func (dto *ExecutionResultDTO) IsFailed() bool {
	return dto.Status == ExecutionStatusFailed
}

// IsComplete returns true if the execution has completed (success or failure).
func (dto *ExecutionResultDTO) IsComplete() bool {
	return dto.Status == ExecutionStatusSuccess ||
		dto.Status == ExecutionStatusFailed ||
		dto.Status == ExecutionStatusSkipped ||
		dto.Status == ExecutionStatusCancelled
}

// MarkStarted marks the execution as started and returns the DTO for method chaining.
func (dto *ExecutionResultDTO) MarkStarted() *ExecutionResultDTO {
	dto.Status = ExecutionStatusRunning
	if dto.StartTime.IsZero() {
		dto.StartTime = time.Now()
	}
	return dto
}

// MarkSuccess marks the execution as successful and returns the DTO for method chaining.
func (dto *ExecutionResultDTO) MarkSuccess() *ExecutionResultDTO {
	dto.Status = ExecutionStatusSuccess
	if dto.EndTime.IsZero() {
		dto.EndTime = time.Now()
	}
	return dto
}

// MarkFailed marks the execution as failed and returns the DTO for method chaining.
func (dto *ExecutionResultDTO) MarkFailed(err error) *ExecutionResultDTO {
	dto.Status = ExecutionStatusFailed
	dto.Error = err
	if dto.EndTime.IsZero() {
		dto.EndTime = time.Now()
	}
	return dto
}

// MarkSkipped marks the execution as skipped and returns the DTO for method chaining.
func (dto *ExecutionResultDTO) MarkSkipped() *ExecutionResultDTO {
	dto.Status = ExecutionStatusSkipped
	if dto.EndTime.IsZero() {
		dto.EndTime = time.Now()
	}
	return dto
}

// MarkCancelled marks the execution as cancelled and returns the DTO for method chaining.
func (dto *ExecutionResultDTO) MarkCancelled() *ExecutionResultDTO {
	dto.Status = ExecutionStatusCancelled
	if dto.EndTime.IsZero() {
		dto.EndTime = time.Now()
	}
	return dto
}

// ExecutionBatchResultDTO represents results for multiple unit executions.
type ExecutionBatchResultDTO struct {
	// Results contains individual unit execution results
	Results []*ExecutionResultDTO

	// Summary provides aggregate statistics
	Summary *ExecutionSummary
}

// ExecutionSummary provides aggregate statistics for a batch of executions.
type ExecutionSummary struct {
	TotalUnits     int
	SuccessCount   int
	FailedCount    int
	SkippedCount   int
	CancelledCount int
	TotalDuration  time.Duration
}

// NewExecutionBatchResultDTO creates a new ExecutionBatchResultDTO.
func NewExecutionBatchResultDTO() *ExecutionBatchResultDTO {
	return &ExecutionBatchResultDTO{
		Results: []*ExecutionResultDTO{},
		Summary: &ExecutionSummary{},
	}
}

// AddResult adds an execution result and updates the summary.
func (dto *ExecutionBatchResultDTO) AddResult(result *ExecutionResultDTO) *ExecutionBatchResultDTO {
	dto.Results = append(dto.Results, result)
	dto.Summary.TotalUnits++

	switch result.Status {
	case ExecutionStatusSuccess:
		dto.Summary.SuccessCount++
	case ExecutionStatusFailed:
		dto.Summary.FailedCount++
	case ExecutionStatusSkipped:
		dto.Summary.SkippedCount++
	case ExecutionStatusCancelled:
		dto.Summary.CancelledCount++
	}

	dto.Summary.TotalDuration += result.Duration()

	return dto
}

// HasFailures returns true if any executions failed.
func (dto *ExecutionBatchResultDTO) HasFailures() bool {
	return dto.Summary.FailedCount > 0
}

// AllSuccessful returns true if all executions were successful.
func (dto *ExecutionBatchResultDTO) AllSuccessful() bool {
	return dto.Summary.TotalUnits > 0 &&
		dto.Summary.SuccessCount == dto.Summary.TotalUnits
}
