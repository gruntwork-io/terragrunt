package azurehelper

import (
	"errors"
	"fmt"
	"testing"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/stretchr/testify/assert"
)

func TestIsAlreadyAssigned(t *testing.T) {
	t.Parallel()

	assert.False(t, isAlreadyAssigned(nil))
	assert.False(t, isAlreadyAssigned(errors.New("RoleAssignmentExists")), "only a service response may match")
	assert.False(t, isAlreadyAssigned(&azcore.ResponseError{ErrorCode: "AuthorizationFailed"}))
	assert.True(t, isAlreadyAssigned(fmt.Errorf("wrapped: %w", &azcore.ResponseError{ErrorCode: "roleassignmentexists"})),
		"the code must match through wrapping and regardless of case")
}
