package cas

import (
	"fmt"
)

// ValidateCASCloneDepth returns an error if depth is not usable for CAS git clones.
// Git requires a positive integer for --depth; zero is invalid. Negative values
// mean full history (Terragrunt omits --depth).
func ValidateCASCloneDepth(depth int) error {
	if depth == 0 {
		return fmt.Errorf(
			"invalid CAS clone depth 0: git clone --depth requires a positive number; "+
				"use the default (%d) or -1 for full history",
			DefaultCASCloneDepth,
		)
	}

	return nil
}
