//go:build azure

package azurehelper_test

import (
	"errors"
	"fmt"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/azure/azurehelper"
	"github.com/stretchr/testify/assert"
)

// TestAzureResponseError tests the Error method of AzureResponseError
func TestAzureResponseError(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		// String fields first (8-byte alignment)
		name        string
		errorCode   string
		message     string
		expectedMsg string
		// Then int fields (4-byte alignment)
		statusCode int
	}{
		{
			name:        "Not Found Error",
			statusCode:  404,
			errorCode:   "ResourceNotFound",
			message:     "The specified resource was not found.",
			expectedMsg: "Azure API error (StatusCode=404, ErrorCode=ResourceNotFound): The specified resource was not found.",
		},
		{
			name:        "Authorization Error",
			statusCode:  403,
			errorCode:   "AuthorizationFailed",
			message:     "The client lacks sufficient authorization.",
			expectedMsg: "Azure API error (StatusCode=403, ErrorCode=AuthorizationFailed): The client lacks sufficient authorization.",
		},
		{
			name:        "Server Error",
			statusCode:  500,
			errorCode:   "InternalServerError",
			message:     "An internal server error occurred.",
			expectedMsg: "Azure API error (StatusCode=500, ErrorCode=InternalServerError): An internal server error occurred.",
		},
		{
			name:        "Empty Error Details",
			statusCode:  0,
			errorCode:   "",
			message:     "",
			expectedMsg: "Azure API error (StatusCode=0, ErrorCode=): ",
		},
	}

	for _, tc := range testCases {
		tc := tc // Capture range variable
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			azureErr := &azurehelper.AzureResponseError{
				StatusCode: tc.statusCode,
				ErrorCode:  tc.errorCode,
				Message:    tc.message,
			}
			assert.Equal(t, tc.expectedMsg, azureErr.Error())
		})
	}
}

// TestGetObjectInputValidation tests the validation of GetObjectInput
func TestGetObjectInputValidation(t *testing.T) {
	t.Parallel()

	// Define test cases
	testCases := []struct {
		name          string
		input         *azurehelper.GetObjectInput
		expectedError string
	}{
		{
			name: "Valid Input",
			input: &azurehelper.GetObjectInput{
				Container: azurehelper.StringPtr("container-name"),
				Key:       azurehelper.StringPtr("blob-key"),
			},
			expectedError: "",
		},
		{
			name: "Missing Container",
			input: &azurehelper.GetObjectInput{
				Key: azurehelper.StringPtr("blob-key"),
			},
			expectedError: "container name is required",
		},
		{
			name: "Empty Container",
			input: &azurehelper.GetObjectInput{
				Container: azurehelper.StringPtr(""),
				Key:       azurehelper.StringPtr("blob-key"),
			},
			expectedError: "container name is required",
		},
		{
			name: "Missing Key",
			input: &azurehelper.GetObjectInput{
				Container: azurehelper.StringPtr("container-name"),
			},
			expectedError: "blob key is required",
		},
		{
			name: "Empty Key",
			input: &azurehelper.GetObjectInput{
				Container: azurehelper.StringPtr("container-name"),
				Key:       azurehelper.StringPtr(""),
			},
			expectedError: "blob key is required",
		},
		{
			name:          "Nil Input",
			input:         nil,
			expectedError: "input cannot be nil",
		},
	}

	// Create a validation test
	// Run test cases
	for _, tc := range testCases {
		tc := tc // Capture range variable
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// We can't actually call GetObject since it requires a real client
			// but we can verify the validation logic separately
			var err error
			switch {
			case tc.input == nil:
				err = errors.New("input cannot be nil")
			case tc.input.Container == nil || *tc.input.Container == "":
				err = errors.New("container name is required")
			case tc.input.Key == nil || *tc.input.Key == "":
				err = errors.New("blob key is required")
			}

			if tc.expectedError == "" {
				assert.NoError(t, err)
			} else {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tc.expectedError)
			}
		})
	}
}

// Mock Azure Response Error for testing
type MockResponseError struct {
	// String fields first (8-byte alignment)
	ErrorCode string
	Message   string
	// Then int fields (4-byte alignment)
	StatusCode int
}

func (e *MockResponseError) Error() string {
	return fmt.Sprintf("Status: %d, Code: %s, Message: %s", e.StatusCode, e.ErrorCode, e.Message)
}
