// Package azurehelper provides Azure-specific helper functions and test utilities
package azurehelper

// StringPtr returns a pointer to the given string value.
// This is a helper function commonly used in tests and when working with Azure SDK functions
// that expect string pointers.
func StringPtr(s string) *string {
	return &s
}
