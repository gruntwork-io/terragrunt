package shell

import (
	"testing"

	"github.com/hashicorp/go-multierror"
	"github.com/stretchr/testify/assert"
)

func TestExplainError(t *testing.T) {
	t.Parallel()

	var testCases = []struct {
		errorOutput string
		explanation string
	}{
		{
			errorOutput: "Error refreshing state: AccessDenied: Access Denied",
			explanation: "Check your credentials and permissions",
		},
		{
			errorOutput: "Error: Initialization required",
			explanation: "You need to run terragrunt (run-all) init to initialize working directory",
		},
		{
			errorOutput: "Module source has changed",
			explanation: "You need to run terragrunt (run-all) init install all required modules",
		},
	}

	for _, tt := range testCases {

		t.Run(tt.errorOutput, func(t *testing.T) {
			err := multierror.Append(&multierror.Error{}, ProcessExecutionError{
				Err:    nil,
				StdOut: "",
				Stderr: tt.errorOutput,
			})
			explanation := ExplainError(err)
			assert.Contains(t, explanation, tt.explanation)

		})
	}

}
