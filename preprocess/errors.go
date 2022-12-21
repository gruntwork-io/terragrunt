package preprocess

import "fmt"

type WrongNumberOfLabels struct {
	blockType          string
	expectedLabelCount int
	actualLabels       []string
}

func (err WrongNumberOfLabels) Error() string {
	return fmt.Sprintf("Expected block of type '%s' to have %d labels, but got %d: %v", err.blockType, err.expectedLabelCount, len(err.actualLabels), err.actualLabels)
}
