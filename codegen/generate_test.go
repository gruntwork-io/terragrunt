package codegen

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRemoteStateConfigToTerraformCode(t *testing.T) {
	expectedOrdered := []byte(`terraform {
  backend "ordered" {
    a = 1
    b = 2
    c = 3
  }
}
`)
	expectedEmpty := []byte(`terraform {
  backend "empty" {
  }
}
`)

	testCases := []struct {
		name     string
		backend  string
		config   map[string]interface{}
		expected []byte
	}{
		{
			"remote-state-config-unsorted-keys",
			"ordered",
			map[string]interface{}{
				"b": 2,
				"a": 1,
				"c": 3,
			},
			expectedOrdered,
		},
		{
			"remote-state-config-sorted-keys",
			"ordered",
			map[string]interface{}{
				"a": 1,
				"b": 2,
				"c": 3,
			},
			expectedOrdered,
		},
		{
			"remote-state-config-empty",
			"empty",
			map[string]interface{}{},
			expectedEmpty,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			actual, err := RemoteStateConfigToTerraformCode(testCase.backend, testCase.config)
			require.True(t, bytes.Contains(actual, []byte(testCase.backend)))
			require.Equal(t, testCase.expected, actual)
			require.Nil(t, err)
		})
	}
}
