package backend_test

import (
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/remotestate/backend"
	"github.com/stretchr/testify/assert"
)

type testConfig struct {
	Name    string `mapstructure:"name"`
	Encrypt bool   `mapstructure:"encrypt"`
	Enabled bool   `mapstructure:"enabled"`
}

type testConfigWithSquash struct {
	Region  string     `mapstructure:"region"`
	Inner   testConfig `mapstructure:",squash"`
	Verbose bool       `mapstructure:"verbose"`
}

func TestNormalizeBoolValues_StringToTrue(t *testing.T) {
	t.Parallel()

	m := backend.Config{"encrypt": "true", "name": "test"}
	result := backend.NormalizeBoolValues(m, &testConfig{})

	assert.Equal(t, true, result["encrypt"])
	assert.Equal(t, "test", result["name"])
}

func TestNormalizeBoolValues_StringToFalse(t *testing.T) {
	t.Parallel()

	m := backend.Config{"encrypt": "false"}
	result := backend.NormalizeBoolValues(m, &testConfig{})

	assert.Equal(t, false, result["encrypt"])
}

func TestNormalizeBoolValues_NativeBoolUnchanged(t *testing.T) {
	t.Parallel()

	m := backend.Config{"encrypt": true, "enabled": false}
	result := backend.NormalizeBoolValues(m, &testConfig{})

	assert.Equal(t, true, result["encrypt"])
	assert.Equal(t, false, result["enabled"])
}

func TestNormalizeBoolValues_NonBoolStringUntouched(t *testing.T) {
	t.Parallel()

	m := backend.Config{"name": "true", "encrypt": "true"}
	result := backend.NormalizeBoolValues(m, &testConfig{})

	// "name" is a string field in the struct, should NOT be converted
	assert.Equal(t, "true", result["name"])
	// "encrypt" is a bool field, should be converted
	assert.Equal(t, true, result["encrypt"])
}

func TestNormalizeBoolValues_InvalidBoolStringLeftAsIs(t *testing.T) {
	t.Parallel()

	m := backend.Config{"encrypt": "maybe"}
	result := backend.NormalizeBoolValues(m, &testConfig{})

	assert.Equal(t, "maybe", result["encrypt"])
}

func TestNormalizeBoolValues_SquashedStructFields(t *testing.T) {
	t.Parallel()

	m := backend.Config{
		"encrypt": "true",
		"enabled": "false",
		"verbose": "true",
		"name":    "test",
		"region":  "us-east-1",
	}
	result := backend.NormalizeBoolValues(m, &testConfigWithSquash{})

	assert.Equal(t, true, result["encrypt"])
	assert.Equal(t, false, result["enabled"])
	assert.Equal(t, true, result["verbose"])
	assert.Equal(t, "test", result["name"])
	assert.Equal(t, "us-east-1", result["region"])
}

func TestNormalizeBoolValues_OriginalMapUnmutated(t *testing.T) {
	t.Parallel()

	m := backend.Config{"encrypt": "true"}
	_ = backend.NormalizeBoolValues(m, &testConfig{})

	// Original map should still have string
	assert.Equal(t, "true", m["encrypt"])
}

func TestNormalizeBoolValues_EmptyMap(t *testing.T) {
	t.Parallel()

	m := backend.Config{}
	result := backend.NormalizeBoolValues(m, &testConfig{})

	assert.Empty(t, result)
}

type testConfigWithPtrBool struct {
	Encrypt  *bool  `mapstructure:"encrypt"`
	Optional *bool  `mapstructure:"optional"`
	Name     string `mapstructure:"name"`
}

func TestNormalizeBoolValues_PtrBoolFields(t *testing.T) {
	t.Parallel()

	m := backend.Config{"encrypt": "true", "optional": "false", "name": "test"}
	result := backend.NormalizeBoolValues(m, &testConfigWithPtrBool{})

	assert.Equal(t, true, result["encrypt"])
	assert.Equal(t, false, result["optional"])
	assert.Equal(t, "test", result["name"])
}

func TestNormalizeBoolValues_NilMap(t *testing.T) {
	t.Parallel()

	result := backend.NormalizeBoolValues(nil, &testConfig{})

	assert.Empty(t, result)
}

func TestNormalizeBoolValues_NilTarget(t *testing.T) {
	t.Parallel()

	m := backend.Config{"encrypt": "true", "name": "test"}
	result := backend.NormalizeBoolValues(m, nil)

	// With nil target, no bool keys are detected, so values are returned as-is
	assert.Equal(t, "true", result["encrypt"])
	assert.Equal(t, "test", result["name"])
}

func TestNormalizeBoolValues_NumericBoolStrings(t *testing.T) {
	t.Parallel()

	m := backend.Config{"encrypt": "1", "enabled": "0"}
	result := backend.NormalizeBoolValues(m, &testConfig{})

	assert.Equal(t, true, result["encrypt"])
	assert.Equal(t, false, result["enabled"])
}
