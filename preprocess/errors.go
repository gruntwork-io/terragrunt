package preprocess

import "fmt"

const bugInTerragrunt = "If the code got here, it suggests a bug in Terragrunt. Please file an issue at: https://github.com/gruntwork-io/terragrunt/issues/."

type WrongNumberOfLabels struct {
	blockType          string
	expectedLabelCount int
	actualLabels       []string
}

func (err WrongNumberOfLabels) Error() string {
	return fmt.Sprintf("Expected block of type '%s' to have %d labels, but got %d: %v", err.blockType, err.expectedLabelCount, len(err.actualLabels), err.actualLabels)
}

type WrongNumberOfArguments struct {
	command  string
	expected int
	actual   int
	usage    string
}

func (err WrongNumberOfArguments) Error() string {
	return fmt.Sprintf("Wrong number of arguments for command '%s': expected %d but got %d. Usage:\n%s", err.command, err.expected, err.actual, err.usage)
}

type UnrecognizedBackendType string

func (err UnrecognizedBackendType) Error() string {
	return fmt.Sprintf("Unrecognized block type for a backend: '%s'. %s", string(err), bugInTerragrunt)
}

type MissingExpectedParam struct {
	param string
	block string
}

func (err MissingExpectedParam) Error() string {
	if err.param == "" {
		return fmt.Sprintf("Could not find block '%s'. %s", err.block, bugInTerragrunt)
	} else {
		return fmt.Sprintf("Could not find param '%s' in block '%s'. %s", err.param, err.block, bugInTerragrunt)
	}
}

type ExceededMaxNestedBlocks int

func (err ExceededMaxNestedBlocks) Error() string {
	return fmt.Sprintf("Hit more than %d levels of nested blocks. Is there any infinite loop somewhere?", int(err))
}

type ResourcesNotAllowed struct {
	resourceAddresses []string
}

func (err ResourcesNotAllowed) Error() string {
	return fmt.Sprintf("Top-level resources are not currently supported. That's because when splitting across multiple environments/modules, it's not clear in which one(s) the resource should go. Found %d resources: %v. Please move these into the relevant modules.", len(err.resourceAddresses), err.resourceAddresses)
}
