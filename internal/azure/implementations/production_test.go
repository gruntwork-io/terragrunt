package implementations_test

import (
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/azure/implementations"
)

// TestProductionServiceContainerCreation tests that the production service container can be created
func TestProductionServiceContainerCreation(t *testing.T) {
	t.Parallel()

	config := map[string]interface{}{
		"subscriptionId": "test-subscription",
	}

	container := implementations.NewProductionServiceContainer(config)

	// Test that container is not nil and implements the interface
	if container == nil {
		t.Fatal("Expected non-nil container")
	}
}

// TestProductionServiceContainerWithNilConfig tests creation with nil config
func TestProductionServiceContainerWithNilConfig(t *testing.T) {
	t.Parallel()

	container := implementations.NewProductionServiceContainer(nil)

	// Test that container is not nil even with nil config
	if container == nil {
		t.Fatal("Expected non-nil container even with nil config")
	}
}
