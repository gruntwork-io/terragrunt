package redact_test

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/gruntwork-io/terragrunt/internal/redact"
)

func TestSecretFormatting(t *testing.T) {
	t.Parallel()

	secret := redact.NewSecret("secret-value")

	for _, verb := range []string{"%s", "%v", "%q"} {
		assert.Equal(t, fmt.Sprintf(verb, redact.Placeholder), fmt.Sprintf(verb, secret), verb)
	}

	assert.Equal(t, redact.Placeholder, fmt.Sprintf("%#v", secret))
}

func TestSecretReveal(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "secret-value", redact.NewSecret("secret-value").Reveal())
}
