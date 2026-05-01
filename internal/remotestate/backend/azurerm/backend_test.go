package azurerm

import "testing"

func TestBackendName(t *testing.T) {
	t.Parallel()

	if got := NewBackend().Name(); got != BackendName {
		t.Fatalf("Name() = %q, want %q", got, BackendName)
	}
}
